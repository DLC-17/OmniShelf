// Package api contains the Gin HTTP handlers for OmniShelf, one file per
// domain, plus the shared plumbing (error envelope, auth middleware, route
// registration) other domains build on.
package api

import "github.com/gin-gonic/gin"

// Machine error codes used in the standard envelope. Keep them stable:
// the frontend switches on these strings.
const (
	CodeInvalidRequest = "invalid_request"
	CodeInviteInvalid  = "invite_invalid"
	CodeInviteUsed     = "invite_used"
	CodeUsernameTaken  = "username_taken"
	CodeBadCredentials = "bad_credentials"
	CodeUnauthorized   = "unauthorized"
	CodeInternal       = "internal_error"
)

// Error writes the uniform API error envelope
// {"error": "<machine_code>", "message": "<human text>"} with the given
// HTTP status.
func Error(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": code, "message": message})
}

// AbortError writes the envelope and aborts the handler chain. Middleware
// must use this variant so downstream handlers never run on failure.
func AbortError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": code, "message": message})
}
