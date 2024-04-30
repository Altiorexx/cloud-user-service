package service

import "os"

type CaseService struct {
	domain string
}

func NewCaseService() *CaseService {
	return &CaseService{
		domain: os.Getenv("CASE_SERVICE_DOMAIN"),
	}
}

func (service *CaseService) GetPermissions() (any, error) {

	return nil, nil
}
