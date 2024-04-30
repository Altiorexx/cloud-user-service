package types

type RegisterUserBody struct {
	Email    string
	Password string
}

type LoginBody struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type RegisterServiceUsedBody struct {
	UserId              string `json:"userId" binding:"required"`
	OrganisationId      string `json:"organisationId" binding:"required"`
	ServiceName         string `json:"serviceName" binding:"required"`
	ImplementationGroup *int   `json:"implementationGroup" binding:"required"`
}
