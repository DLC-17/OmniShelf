package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exhausting the failure budget with wrong passwords must lock the source IP
// out of login (429), and a valid session must not be issued afterwards.
func TestLoginRateLimitedAfterRepeatedFailures(t *testing.T) {
	r, gdb := newTestRouter(t)
	seedInvite(t, gdb, "CODE-RATE")
	require.Equal(t, http.StatusCreated,
		doJSON(r, http.MethodPost, "/api/auth/register", registerBody("alice", "CODE-RATE")).Code)

	bad := map[string]string{"username": "alice", "password": "wrong-password"}
	for i := range authFailureLimit {
		w := doJSON(r, http.MethodPost, "/api/auth/login", bad)
		require.Equal(t, http.StatusUnauthorized, w.Code, "attempt %d", i)
	}

	// Budget exhausted: even the correct password is rejected with 429 now.
	w := doJSON(r, http.MethodPost, "/api/auth/login",
		map[string]string{"username": "alice", "password": "hunter2hunter2"})
	require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
	assertEnvelope(t, w, CodeRateLimited)
}

// Failed invite-code guesses consume the same per-IP budget on register.
func TestRegisterRateLimitedAfterRepeatedInviteGuesses(t *testing.T) {
	r, _ := newTestRouter(t)

	for i := range authFailureLimit {
		w := doJSON(r, http.MethodPost, "/api/auth/register", registerBody("mallory", "WRONG-GUESS"))
		require.Equal(t, http.StatusBadRequest, w.Code, "attempt %d", i)
	}

	w := doJSON(r, http.MethodPost, "/api/auth/register", registerBody("mallory", "WRONG-GUESS"))
	require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
	assertEnvelope(t, w, CodeRateLimited)
}

// A successful login clears the counter so a legitimate user who mistypes a
// few times is not penalized on later attempts.
func TestLoginSuccessResetsFailureBudget(t *testing.T) {
	l := newFailureLimiter(3, time.Minute)
	l.fail("ip")
	l.fail("ip")
	assert.False(t, l.blocked("ip"))
	l.reset("ip")
	l.fail("ip")
	l.fail("ip")
	assert.False(t, l.blocked("ip"))
	l.fail("ip")
	assert.True(t, l.blocked("ip"))
}
