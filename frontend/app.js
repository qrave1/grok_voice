/**
 * Генерация простого уникального идентификатора для клиента.
 * В реальном проекте можно использовать UUID.
 */
function generateClientId() {
    return 'client-' + Math.random().toString(36).substring(2, 10);
}

/**
 * Alpine.js компонент с основной логикой:
 * - Авторизация (login/register) через REST.
 * - Получение списка комнат (GET /rooms).
 * - При выборе комнаты – подключение по WebSocket и установление WebRTC соединения.
 * - Обновление списка участников и информации о комнате.
 */
function app() {
    return {
        // Состояние приложения
        view: "auth",          // "auth" или "main"
        authMode: "login",     // "login" или "register"
        username: "",
        password: "",
        clientId: generateClientId(),

        // Данные для работы с комнатами
        rooms: [],
        selectedRoom: null,
        participants: [],      // массив строк с идентификаторами участников
        roomInfo: {},          // объект с информацией о комнате (например, { id, creator })

        // WebSocket и WebRTC
        ws: null,
        localStream: null,
        peerConnection: null,

        /**
         * Логин пользователя через REST (POST /login).
         * При успехе переключаем вид на "main" и инициализируем WebSocket.
         */
        login() {
            fetch("/login", {
                method: "POST",
                headers: {"Content-Type": "application/json"},
                body: JSON.stringify({username: this.username, password: this.password})
            })
                .then(response => {
                    if (response.ok) {
                        this.view = "main";
                        this.initWs();
                        this.fetchRooms();
                    } else {
                        alert("Ошибка входа. Проверьте имя пользователя и пароль.");
                    }
                })
                .catch(err => {
                    console.error(err);
                    alert("Ошибка запроса.");
                });
        },

        /**
         * Регистрация пользователя через REST (POST /register).
         * При успехе переключаем вид на "main" и инициализируем WebSocket.
         */
        register() {
            fetch("/register", {
                method: "POST",
                headers: {"Content-Type": "application/json"},
                body: JSON.stringify({username: this.username, password: this.password})
            })
                .then(response => {
                    if (response.ok) {
                        this.view = "main";
                        this.initWs();
                        this.fetchRooms();
                    } else {
                        alert("Ошибка регистрации. Возможно, пользователь уже существует.");
                    }
                })
                .catch(err => {
                    console.error(err);
                    alert("Ошибка запроса.");
                });
        },

        /**
         * Получение списка комнат с бэкенда (GET /rooms).
         */
        fetchRooms() {
            fetch("/rooms", {method: "GET"})
                .then(response => response.json())
                .then(data => {
                    // Ожидается, что data — массив объектов комнаты,
                    // например: [{ id: "room1", name: "Общий", creator: "admin" }, …]
                    this.rooms = data;
                })
                .catch(err => {
                    console.error(err);
                    alert("Ошибка получения списка комнат.");
                });
        },

        /**
         * Инициализация WebSocket-соединения.
         */
        initWs() {
            if (this.ws) return;
            this.ws = new WebSocket(`ws://${window.location.host}/ws`);
            this.ws.onopen = () => {
                console.log("WebSocket: соединение установлено");
            };
            this.ws.onmessage = (event) => {
                const msg = JSON.parse(event.data);
                // Обработка сигналов от сервера
                if (msg.type === "answer" && msg.sdp) {
                    this.handleAnswer(msg.sdp);
                } else if (msg.type === "candidate" && msg.candidate) {
                    this.handleCandidate(msg.candidate);
                } else if (msg.type === "participants" && msg.participants) {
                    // Обновляем список участников для выбранной комнаты
                    this.participants = msg.participants;
                    // Можно обновить также roomInfo, если сервер передает доп. данные
                    if (msg.roomInfo) {
                        this.roomInfo = msg.roomInfo;
                    }
                } else if (msg.type === "error") {
                    alert("Ошибка: " + msg.message);
                }
            };
            this.ws.onclose = () => {
                console.log("WebSocket: соединение закрыто");
            };
            this.ws.onerror = (err) => {
                console.error("WebSocket ошибка:", err);
            };
        },

        /**
         * Отправка сообщения через WebSocket.
         */
        sendWsMessage(msg) {
            if (this.ws && this.ws.readyState === WebSocket.OPEN) {
                this.ws.send(JSON.stringify(msg));
            } else {
                console.error("WebSocket не подключен");
            }
        },

        /**
         * При выборе комнаты:
         * - Сохраняем выбранную комнату
         * - Отправляем на сервер сообщение join (с указанием roomId и clientId)
         * - Запускаем голосовое соединение (WebRTC)
         */
        selectRoom(room) {
            this.selectedRoom = room;
            // Очистка предыдущего списка участников
            this.participants = [];
            // Отправляем join-сообщение
            this.sendWsMessage({
                type: "join",
                roomId: room.id,
                clientId: this.clientId
            });
            // Сохраняем инфо о комнате для правой колонки
            this.roomInfo = {id: room.id, creator: room.creator};
            // Запускаем голосовое соединение
            this.startVoiceCall();
        },

        /**
         * Начало голосового вызова.
         * Запрашиваем аудио, создаем RTCPeerConnection, добавляем дорожки и отправляем SDP offer.
         */
        async startVoiceCall() {
            try {
                const stream = await navigator.mediaDevices.getUserMedia({audio: true});
                this.localStream = stream;
                const rtcConfig = {iceServers: [{urls: "stun:stun.l.google.com:19302"}]};
                this.peerConnection = new RTCPeerConnection(rtcConfig);

                // Добавляем аудиодорожки
                stream.getTracks().forEach(track => {
                    this.peerConnection.addTrack(track, stream);
                });

                // Обработка ICE-кандидатов
                this.peerConnection.onicecandidate = (event) => {
                    if (event.candidate) {
                        this.sendWsMessage({
                            type: "candidate",
                            candidate: event.candidate,
                            roomId: this.selectedRoom.id,
                            clientId: this.clientId
                        });
                    }
                };

                // При получении аудио устанавливаем поток в audio-элемент
                this.peerConnection.ontrack = (event) => {
                    const remoteAudio = document.getElementById("remoteAudio");
                    remoteAudio.srcObject = event.streams[0];
                };

                // Создаем SDP предложение
                const offer = await this.peerConnection.createOffer();
                await this.peerConnection.setLocalDescription(offer);
                // Отправляем SDP предложение серверу
                this.sendWsMessage({
                    type: "offer",
                    sdp: offer,
                    roomId: this.selectedRoom.id,
                    clientId: this.clientId
                });
            } catch (e) {
                console.error("Ошибка при запуске вызова:", e);
                alert("Ошибка при запуске голосового вызова");
            }
        },

        /**
         * Обработка SDP answer от сервера.
         */
        async handleAnswer(sdp) {
            if (!this.peerConnection) {
                console.error("RTCPeerConnection не создан");
                return;
            }
            try {
                // Если sdp является обычным объектом, оборачиваем его в RTCSessionDescription
                const remoteDesc = new RTCSessionDescription(sdp);
                await this.peerConnection.setRemoteDescription(remoteDesc);
                console.log("Remote description успешно установлена");
            } catch (e) {
                console.error("Ошибка установки remote description", e);
            }
        },

        /**
         * Обработка ICE-кандидата, полученного от сервера.
         */
        async handleCandidate(candidate) {
            if (!this.peerConnection) {
                console.error("RTCPeerConnection не создан");
                return;
            }
            try {
                // Если candidate является обычным объектом, оборачиваем его в RTCIceCandidate
                const iceCandidate = new RTCIceCandidate(candidate);
                await this.peerConnection.addIceCandidate(iceCandidate);
                console.log("ICE кандидат успешно добавлен");
            } catch (e) {
                console.error("Ошибка добавления ICE кандидата", e);
            }
        },
    };
}
