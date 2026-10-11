// Package ports defines dependencies required by the business services.
package ports

import (
	"context"
	"github.com/barlus-developer/phajay-payment-gateway-sandbox-server/internal/domain"
)

type Repository interface {
	Dashboard(context.Context, string, int64) (domain.Dashboard, error)
	CreateDeveloper(context.Context, domain.Developer) error
	DeveloperByEmail(context.Context, string) (domain.Developer, error)
	DeveloperByID(context.Context, string) (domain.Developer, error)
	UpdateDeveloper(context.Context, string, string) error
	ChangePassword(context.Context, string, string, string) error
	CreateSession(context.Context, domain.Session) error
	Session(context.Context, string, int64) (domain.Session, error)
	Sessions(context.Context, string, int64) ([]domain.Session, error)
	RevokeSession(context.Context, string, string) error
	CreateProject(context.Context, domain.Project) error
	Projects(context.Context, string) ([]domain.Project, error)
	Project(context.Context, string, string) (domain.Project, error)
	UpdateProject(context.Context, domain.Project) error
	CreateAPIKey(context.Context, domain.APIKey, string) error
	APIKeys(context.Context, string) ([]domain.APIKey, error)
	AuthenticateKey(context.Context, string, int64) (domain.Project, error)
	RevokeAPIKey(context.Context, string, string, int64) error
	RotateSigningKey(context.Context, domain.SigningKey, int64) error
	SigningKeys(context.Context, string) ([]domain.SigningKey, error)
	ActiveSigningKey(context.Context, string) (domain.SigningKey, error)
	RevokeSigningKey(context.Context, string, string, int64) error
	SaveEndpoint(context.Context, domain.Endpoint) error
	Endpoints(context.Context, string) ([]domain.Endpoint, error)
	DeleteEndpoint(context.Context, string, string) error
	CreateTransaction(context.Context, domain.Transaction, domain.TransactionEvent) error
	Transaction(context.Context, string, string) (domain.Transaction, error)
	TransactionByToken(context.Context, string) (domain.Transaction, error)
	Transactions(context.Context, string, domain.Filter) ([]domain.Transaction, int, error)
	TransactionEvents(context.Context, string) ([]domain.TransactionEvent, error)
	Transition(context.Context, string, domain.State, int64) (domain.Transaction, bool, error)
	Expiring(context.Context, int64) ([]domain.Transaction, error)
	Enqueue(context.Context, domain.OutboxEvent, string) error
	ClaimJob(context.Context, int64, int64) (domain.Job, error)
	FinishJob(context.Context, domain.Job, domain.Delivery, bool, int64) error
	Jobs(context.Context, string, int, int) ([]domain.Job, error)
	Deliveries(context.Context, string, string) ([]domain.Delivery, error)
	RetryJob(context.Context, string, string, int64) error
	WriteLog(context.Context, domain.APILog) error
	Logs(context.Context, string, int, int) ([]domain.APILog, error)
	Ping(context.Context) error
}
type Sender interface {
	Validate(context.Context, string) error
	Send(context.Context, string, []byte, string, string) (int, string, error)
}
