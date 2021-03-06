// Copyright 2017, Square, Inc.

// Package api provides controllers for each API endpoint. Controllers are
// "dumb wiring"; there is little to no application logic in this package.
// Controllers call and coordinate other packages to satisfy the API endpoint.
package api

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/square/spincycle/job-runner/chain"
	"github.com/square/spincycle/job-runner/runner"
	"github.com/square/spincycle/proto"
	"github.com/square/spincycle/router"
)

const (
	API_ROOT           = "/api/v1/"
	REQUEST_ID_PATTERN = "([0-9]+)"
)

// API provides controllers for endpoints it registers with a router.
type API struct {
	Router        *router.Router
	chainRepo     chain.Repo
	runnerFactory runner.RunnerFactory
	traverserRepo chain.TraverserRepo // Repo for keeping track of active traversers
}

var hostname func() (string, error) = os.Hostname

// NewAPI makes a new API.
func NewAPI(router *router.Router, chainRepo chain.Repo, runnerFactory runner.RunnerFactory) *API {
	api := &API{
		Router:        router,
		chainRepo:     chainRepo,
		runnerFactory: runnerFactory,
		traverserRepo: chain.NewTraverserRepo(),
	}

	api.Router.AddRoute(API_ROOT+"job-chains", api.newJobChainHandler, "api-new-job-chain")
	api.Router.AddRoute(API_ROOT+"job-chains/"+REQUEST_ID_PATTERN+"/start", api.startJobChainHandler, "api-start-job-chain")
	api.Router.AddRoute(API_ROOT+"job-chains/"+REQUEST_ID_PATTERN+"/stop", api.stopJobChainHandler, "api-stop-job-chain")
	api.Router.AddRoute(API_ROOT+"job-chains/"+REQUEST_ID_PATTERN+"/status", api.statusJobChainHandler, "api-status-job-chain")

	return api
}

// ============================== CONTROLLERS ============================== //

// POST <API_ROOT>/job-chains
// Do some basic validation on a job chain, and, if it passes, add it to the
// chain repo. If it doesn't pass, return the validation error.
func (api *API) newJobChainHandler(ctx router.HTTPContext) {
	switch ctx.Request.Method {
	case "POST":
		decoder := json.NewDecoder(ctx.Request.Body)
		var jobChain proto.JobChain
		err := decoder.Decode(&jobChain)
		if err != nil {
			ctx.APIError(router.ErrInternal, "Can't decode request body (error: %s)", err)
			return
		}

		c := chain.NewChain(&jobChain)
		requestIdStr := strconv.FormatUint(uint64(c.RequestId()), 10)

		// Create a new traverser.
		traverser, err := chain.NewTraverser(api.chainRepo, api.runnerFactory, c)
		if err != nil {
			ctx.APIError(router.ErrBadRequest, "Problem creating traverser (error: %s)", err)
			return
		}

		// Add the traverser to the repo.
		err = api.traverserRepo.Add(requestIdStr, traverser)
		if err != nil {
			ctx.APIError(router.ErrBadRequest, err.Error())
			return
		}
	default:
		ctx.UnsupportedAPIMethod()
	}
}

// PUT <API_ROOT>/job-chains/{requestId}/start
// Start the traverser for a job chain.
func (api *API) startJobChainHandler(ctx router.HTTPContext) {
	switch ctx.Request.Method {
	case "PUT":
		requestIdStr := ctx.Arguments[1]

		// Get the traverser from the repo.
		traverser, err := api.traverserRepo.Get(requestIdStr)
		if err != nil {
			ctx.APIError(router.ErrNotFound, "Can't retrieve traverser from repo (error: %s).", err.Error())
			return
		}

		// Set the location in the response header to point to this server.
		ctx.Response.Header().Set("Location", chainLocation(requestIdStr, os.Hostname))

		// Start the traverser, and remove it from the repo when it's
		// done running. This could take a very long time to return,
		// so we run it in a goroutine.
		go func() {
			traverser.Run()
			api.traverserRepo.Remove(requestIdStr)
		}()
	default:
		ctx.UnsupportedAPIMethod()
	}
}

// PUT <API_ROOT>/job-chains/{requestId}/stop
// Stop the traverser for a job chain.
func (api *API) stopJobChainHandler(ctx router.HTTPContext) {
	switch ctx.Request.Method {
	case "PUT":
		requestIdStr := ctx.Arguments[1]

		// Get the traverser to the repo.
		traverser, err := api.traverserRepo.Get(requestIdStr)
		if err != nil {
			ctx.APIError(router.ErrNotFound, "Can't retrieve traverser from repo (error: %s).", err.Error())
			return
		}

		// This is expected to return quickly.
		err = traverser.Stop()
		if err != nil {
			ctx.APIError(router.ErrInternal, "Can't stop the chain (error: %s)", err)
			return
		}

		api.traverserRepo.Remove(requestIdStr)
	default:
		ctx.UnsupportedAPIMethod()
	}
}

// GET <API_ROOT>/job-chains/{requestId}/status
// Get the status of a running job chain.
func (api *API) statusJobChainHandler(ctx router.HTTPContext) {
	switch ctx.Request.Method {
	case "GET":
		requestIdStr := ctx.Arguments[1]

		// Get the traverser to the repo.
		traverser, err := api.traverserRepo.Get(requestIdStr)
		if err != nil {
			ctx.APIError(router.ErrNotFound, "Can't retrieve traverser from repo (error: %s).", err.Error())
			return
		}

		// This is expected to return quickly.
		statuses, err := traverser.Status()
		if err != nil {
			ctx.APIError(router.ErrInternal, "Can't get the chain's status (error: %s)", err)
			return
		}

		if out, err := marshal(statuses); err != nil {
			ctx.APIError(router.ErrInternal, "Can't encode response (error: %s)", err)
		} else {
			fmt.Fprintln(ctx.Response, string(out))
		}
	default:
		ctx.UnsupportedAPIMethod()
	}
}

// ========================================================================= //

// chainLocation returns the URL location of a job chain
func chainLocation(requestId string, hostname func() (string, error)) string {
	h, _ := hostname()
	return h + API_ROOT + "job-chains/" + requestId
}

// marshal is a helper function to nicely print JSON.
func marshal(v interface{}) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}
