package usecase

// TODO REG
//hashedPassword, err := bcrypt.GenerateFromPassword([]byte(creds.Password), bcrypt.DefaultCost)
//if err != nil {
//	http.Error(w, "Server error", http.StatusInternalServerError)
//	return
//}
//
//user := domain.NewUser(creds.Username, string(hashedPassword))
//err = db.QueryRow(
//	"INSERT INTO users (username, password) VALUES ($1, $2) RETURNING id",
//	creds.Username,
//	hashedPassword,
//).Scan(&userID)
//if err != nil {
//	slog.Error("register user", "error", err)
//	http.Error(w, "User already exists or database error", http.StatusConflict)
//	return
//}
//slog.Info("User registered", "username", creds.Username)
//token, err := uh.tm.Generate(user.ID)
//if err != nil {
//	http.Error(w, "generate token", http.StatusInternalServerError)
//	return
//}

// TODO LOGIN
//var user domain.User
//err := db.Get(&user, "SELECT * FROM users WHERE username=$1", creds.Username)
//if err != nil {
//	slog.Error("User not found", "username", creds.Username, "error", err)
//	http.Error(w, "Invalid credentials", http.StatusUnauthorized)
//	return
//}
//
//if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(creds.Password)); err != nil {
//	slog.Error("Invalid password", "username", creds.Username)
//	http.Error(w, "Invalid credentials", http.StatusUnauthorized)
//	return
//}
//
//token, err := generateJWT(user.ID)
//if err != nil {
//	http.Error(w, "generate token", http.StatusInternalServerError)
//	return
//}
//slog.Info("User logged in", "username", creds.Username)
