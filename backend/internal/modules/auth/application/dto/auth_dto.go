package dto

// LoginCommand is the POST /login request body.
type LoginCommand struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// LoginResponse carries access + refresh tokens and basic user info.
// The React Router BFF stores refreshToken in an HttpOnly cookie; the browser
// only keeps accessToken in memory.
type LoginResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	TokenType    string `json:"tokenType"`
	UserID       string `json:"userId"`
	UserName     string `json:"userName"`
}

// RefreshCommand is the POST /refresh, /logout, /session, and /session/access body.
type RefreshCommand struct {
	RefreshToken string `json:"refreshToken" validate:"required"`
}

// ServerAccessResponse is POST /session/access: mint a new access token only.
// The refresh session is not rotated. User fields support browser cold bootstrap
// after silent renew (memory AT gone on reload).
type ServerAccessResponse struct {
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType"`
	UserID      string `json:"userId"`
	UserName    string `json:"userName"`
}
