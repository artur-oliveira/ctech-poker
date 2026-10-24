package v1

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"gopkg.aoctech.app/api-commons/problem"
	"gopkg.aoctech.app/poker/api/internal/responsible"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

// pokerTypes registers the problem types a client branches on. The slugs keep
// the "/problems/..." URIs the UI and the wallet-era clients already know.
var pokerTypes = problem.NewRegistry(problem.DefaultBaseURI)

const (
	slugTableFull                = "table-full"
	slugGamblingTermsRequired    = "gambling-terms-required"
	slugSelfExcluded             = "self-excluded"
	slugLimitsNotConfigured      = "limits-not-configured"
	slugDepositLimitExceeded     = "deposit-limit-exceeded"
	slugExclusionChangeRejected  = "exclusion-change-rejected"
	timeoutDetail                = "the request took too long; try again"
	limiterTooManyRequestsDetail = "too many requests, slow down"
)

var (
	_ = pokerTypes.Register(slugTableFull, http.StatusConflict, "Table Full")
	_ = pokerTypes.Register(slugGamblingTermsRequired, http.StatusForbidden, "Gambling Terms Required")
	_ = pokerTypes.Register(slugSelfExcluded, http.StatusConflict, "Self-Excluded")
	_ = pokerTypes.Register(slugLimitsNotConfigured, http.StatusConflict, "Limits Not Configured")
	_ = pokerTypes.Register(slugDepositLimitExceeded, http.StatusConflict, "Deposit Limit Exceeded")
	_ = pokerTypes.Register(slugExclusionChangeRejected, http.StatusConflict, "Change Rejected")
)

// unauthorizedProblem carries NextActionReauthenticate (#319): an invalid or
// expired token is fixed by refreshing it or signing in again, not by retrying.
func unauthorizedProblem(detail string) *problem.Problem {
	return problem.Unauthorized(detail).WithNextAction(problem.NextActionReauthenticate, 0)
}

// internalProblem is the one place handlers turn an unexpected error into a
// response, so a call cut by the request's time budget answers a retryable 504
// everywhere.
func internalProblem(detail string, err error) *problem.Problem {
	if errors.Is(err, context.DeadlineExceeded) {
		return problem.GatewayTimeout(timeoutDetail).WithCause(err)
	}
	return problem.InternalServer(detail).WithCause(err)
}

// tableFullProblem carries NextActionRetry (#319): the lost seat may free up
// again, so retrying (e.g. another table) is the client's correct move.
func tableFullProblem() *problem.Problem {
	return pokerTypes.New(slugTableFull, "the last available seat was taken").
		WithNextAction(problem.NextActionRetry, 0)
}

// walletProblem passes ctech-wallet's own problem+json straight through when
// err wraps a *walletclient.Error, so the wrapped Go error chain never reaches
// the response detail. ok is false when err is not a wallet error.
func walletProblem(err error) (p *problem.Problem, ok bool) {
	var werr *walletclient.Error
	if !errors.As(err, &werr) {
		return nil, false
	}
	p = problem.New(werr.Status, werr.Type, werr.Title, werr.Detail)
	// #319: propagate the wallet's own next_action/retry_after_seconds verbatim.
	if werr.NextAction != "" {
		p = p.WithNextAction(werr.NextAction, werr.RetryAfterSeconds)
	}
	return p, true
}

var depositWindowNames = map[string]string{"daily": "diário", "weekly": "semanal", "monthly": "mensal"}

// responsibleProblem maps a responsible-gambling refusal to its problem+json;
// ok is false for any other error.
func responsibleProblem(err error) (p *problem.Problem, ok bool) {
	var (
		excluded *responsible.ExcludedError
		breach   *responsible.BreachError
		rule     *responsible.RuleError
	)
	switch {
	case errors.Is(err, responsible.ErrConsentRequired):
		return pokerTypes.New(slugGamblingTermsRequired,
			"aceite o termo de jogo responsável para jogar com dinheiro real"), true
	case errors.Is(err, responsible.ErrLimitsNotConfigured):
		return pokerTypes.New(slugLimitsNotConfigured,
			"defina seus limites (diário, semanal e mensal) antes de jogar com dinheiro real"), true
	case errors.As(err, &excluded):
		detail := "você se autoexcluiu do jogo por tempo indeterminado"
		if excluded.Until != "" {
			detail = "você se autoexcluiu do jogo até " + excluded.Until
		}
		return pokerTypes.New(slugSelfExcluded, detail), true
	case errors.As(err, &breach):
		return pokerTypes.New(slugDepositLimitExceeded,
			fmt.Sprintf("limite %s atingido; renova em %s", depositWindowNames[breach.Breach.Window], breach.Breach.ResetsAt.Format(time.RFC3339))), true
	case errors.As(err, &rule):
		return pokerTypes.New(slugExclusionChangeRejected, rule.Message), true
	}
	return nil, false
}
