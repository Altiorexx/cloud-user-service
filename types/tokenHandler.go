package types

type ValidateTokenBody struct {
	Token string `json:"token" binding:"required"`
}

type TokenData struct {
	UserId          string   `json:"userId" binding:"required"`
	OrganisationIds []string `json:"organisationIds"`
}
