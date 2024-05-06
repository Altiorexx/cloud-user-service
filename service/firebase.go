package service

import (
	"context"
	"fmt"
	"sync"

	firebase "firebase.google.com/go"
	"firebase.google.com/go/auth"

	"google.golang.org/api/option"
)

var (
	firebase_once     sync.Once
	firebase_instance *FirebaseService
)

type FirebaseService struct {
	auth  *auth.Client
	email *EmailService
}

func NewFirebaseService() *FirebaseService {
	firebase_once.Do(func() {

		//option.WithCredentialsJSON()
		opt := option.WithCredentialsFile("./cloud-421916-firebase-adminsdk-r2o16-4f7e7089fe.json")
		app, err := firebase.NewApp(context.Background(), nil, opt)
		if err != nil {
			panic(fmt.Errorf("error initializing app: %+v", err))
		}

		auth, err := app.Auth(context.Background())
		if err != nil {
			panic(fmt.Errorf("error instantiating app: %+v", err))
		}

		firebase_instance = &FirebaseService{
			auth:  auth,
			email: NewEmailService(),
		}
	})

	// return reference
	return firebase_instance
}

// Verifies a token through Firebase, returns the decoded token if valid.
func (service *FirebaseService) VerifyToken(token string) (*auth.Token, error) {
	decodedToken, err := service.auth.VerifyIDTokenAndCheckRevoked(context.Background(), token)
	if err != nil {
		return nil, err
	}
	return decodedToken, nil
}

// Revokes a user's refresh token.
func (service *FirebaseService) RevokeToken(uid string) error {
	return service.auth.RevokeRefreshTokens(context.Background(), uid)
}

func (service *FirebaseService) InviteMember(organisationId string, email string) error {

	// generate link
	link, err := service.auth.EmailSignInLink(context.Background(), email, &auth.ActionCodeSettings{
		URL: fmt.Sprintf("http://localhost:2000/signup?o=%s", organisationId),
	})
	if err != nil {
		return err
	}

	// generate template and send mail
	message := service.email.CreateInvitationMail(email, link)
	if err := service.email.Send([]string{email}, message); err != nil {
		return err
	}

	return nil
}

// Create a user in firebase.
func (service *FirebaseService) CreateUser(email string, password string, name string) (string, error) {
	params := (&auth.UserToCreate{}).Email(email).Password(password).DisplayName(name)
	user, err := service.auth.CreateUser(context.Background(), params)
	if err != nil {
		return "", err
	}
	return user.UID, nil
}

// Delete a user in firebase.
func (service *FirebaseService) DeleteUser(userId string) error {
	return service.auth.DeleteUser(context.Background(), userId)
}
