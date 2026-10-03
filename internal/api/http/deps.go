package http

import (
	"github.com/example/hotel-booking/internal/api/http/handler"
	"github.com/example/hotel-booking/internal/api/http/middleware"
)

// Deps adalah alias ke handler.Deps untuk konfigurasi dependency injection HTTP transport.
type Deps = handler.Deps

// Re-export tipe middleware yang lazim digunakan oleh caller HTTP.
type (
	RateLimiter      = middleware.RateLimiter
	StaffAuthService = middleware.StaffAuthService
	StaffVerifier    = middleware.StaffVerifier
	AuthContext      = middleware.AuthContext
	ProblemDetails   = middleware.ProblemDetails
	GuestContextKey  = middleware.GuestContextKey
)

const (
	DefaultMaxBodyBytes = middleware.DefaultMaxBodyBytes
	RoleKey             = middleware.RoleKey
	SubjectKey          = middleware.SubjectKey
	GuestTokenKey       = middleware.GuestTokenKey
)

var (
	TestStaffVerifier       = middleware.TestStaffVerifier
	NewRateLimiter          = middleware.NewRateLimiter
	WriteProblemDetails     = middleware.WriteProblemDetails
	WriteError              = middleware.WriteError
	WriteJSON               = middleware.WriteJSON
	HttpErrorCode           = middleware.HttpErrorCode
	BodySizeLimit           = middleware.BodySizeLimit
	RequestID               = middleware.RequestID
	AccessLog               = middleware.AccessLog
	RecoverProblem          = middleware.RecoverProblem
	SecureHeaders           = middleware.SecureHeaders
	NoStore                 = middleware.NoStore
	TimeoutContext          = middleware.TimeoutContext
	NotFound                = middleware.NotFound
	MethodNotAllowed        = middleware.MethodNotAllowed
	CORS                    = middleware.CORS
	IdentifySubject         = middleware.IdentifySubject
	RequireStaffSession     = middleware.RequireStaffSession
	Authorize               = middleware.Authorize
	RequireGuestSession     = middleware.RequireGuestSession
	GuestSessionFromContext = middleware.GuestSessionFromContext
	RequireFeature          = middleware.RequireFeature
	FeatureEnabled          = middleware.FeatureEnabled
	WithIdempotency         = middleware.WithIdempotency
	GetAuthContext          = middleware.GetAuthContext
)

type SearchResultItem = handler.SearchResultItem
