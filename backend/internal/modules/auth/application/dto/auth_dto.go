package dto

// LoginCommand is the POST /login request body.
type LoginCommand struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginResponse carries the issued JWT and basic user info.
type LoginResponse struct {
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType"`
	UserID      string `json:"userId"`
}
