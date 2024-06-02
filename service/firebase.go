package service

import (
	"context"
	"fmt"
	"sync"

	firebase "firebase.google.com/go"
	"firebase.google.com/go/auth"

	"google.golang.org/api/option"
)

type FirebaseService interface {
	VerifyToken(token string) (*auth.Token, error)
	SetNewPassword(uid string, password string) error
	ResetPassword(email string) (string, error)
	RevokeToken(uid string) error
	UserExists(email string) error
	GetUserIdByEmail(email string) (string, error)
	InviteMember(organisationId string, email string) error
	CreateUser(email string, password string, name string) (string, error)
	DeleteUser(userId string) error
}

type FirebaseServiceOpts struct {
	Email EmailService
}

var (
	firebase_service_instance_map = make(map[string]*FirebaseServiceImpl)
	mu                            sync.Mutex
)

type FirebaseServiceImpl struct {
	auth  *auth.Client
	email EmailService
}

func NewFirebaseService(opts *FirebaseServiceOpts, key string) *FirebaseServiceImpl {

	mu.Lock()
	defer mu.Unlock()

	if instance, exists := firebase_service_instance_map[key]; exists {
		return instance
	}

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

	firebase_service_instance_map[key] = &FirebaseServiceImpl{
		auth:  auth,
		email: opts.Email,
	}

	return firebase_service_instance_map[key]
}

// Verifies a token through Firebase, returns the decoded token if valid.
func (service *FirebaseServiceImpl) VerifyToken(token string) (*auth.Token, error) {
	decodedToken, err := service.auth.VerifyIDTokenAndCheckRevoked(context.Background(), token)
	if err != nil {
		return nil, err
	}
	return decodedToken, nil
}

// Set a user's password.
func (service *FirebaseServiceImpl) SetNewPassword(uid string, password string) error {
	changes := &auth.UserToUpdate{}
	changes.Password(password)
	_, err := service.auth.UpdateUser(context.Background(), uid, changes)
	return err
}

// Allow the user to reset their password through firebase.
func (service *FirebaseServiceImpl) ResetPassword(email string) (string, error) {
	return service.auth.PasswordResetLink(context.Background(), email)
}

// Revokes a user's refresh token.
func (service *FirebaseServiceImpl) RevokeToken(uid string) error {
	return service.auth.RevokeRefreshTokens(context.Background(), uid)
}

// Check if a user exists by email
func (service *FirebaseServiceImpl) UserExists(email string) error {
	_, err := service.auth.GetUserByEmail(context.Background(), email)
	return err
}

// Get userId by email.
func (service *FirebaseServiceImpl) GetUserIdByEmail(email string) (string, error) {
	user, err := service.auth.GetUserByEmail(context.Background(), email)
	if err != nil {
		return "", err
	}
	return user.UID, nil
}

func (service *FirebaseServiceImpl) InviteMember(organisationId string, email string) error {

	// generate link
	link, err := service.auth.EmailSignInLink(context.Background(), email, &auth.ActionCodeSettings{
		URL: fmt.Sprintf("http://localhost:2000/signup?o=%s", organisationId),
	})
	if err != nil {
		return err
	}

	// generate template and send mail
	message := service.email.CreateInvitationMail(email, link, "")
	if err := service.email.Send([]string{email}, message); err != nil {
		return err
	}

	return nil
}

// Create a user in firebase.
func (service *FirebaseServiceImpl) CreateUser(email string, password string, name string) (string, error) {
	params := (&auth.UserToCreate{}).Email(email).Password(password).DisplayName(name)
	user, err := service.auth.CreateUser(context.Background(), params)
	if err != nil {
		return "", err
	}
	return user.UID, nil
}

// Delete a user in firebase.
func (service *FirebaseServiceImpl) DeleteUser(userId string) error {
	return service.auth.DeleteUser(context.Background(), userId)
}
