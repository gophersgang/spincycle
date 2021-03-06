// Copyright 2017, Square, Inc.

package runner

import (
	"github.com/square/spincycle/job"
)

// A RunnerFactory makes a Runner for one job of the given type, re-created
// with the given bytes, and associated with the given request ID. The job
// name is only used for testing with a mock RunnerFactory. An error is returned
// if the job fails to instantiate or re-create itself.
type RunnerFactory interface {
	Make(jobType, jobName string, jobBytes []byte, requestId uint) (Runner, error)
}

type runnerFactory struct {
	jobFactory job.Factory
}

// NewRunnerFactory makes a RunnerFactory.
func NewRunnerFactory(jobFactory job.Factory) RunnerFactory {
	return &runnerFactory{
		jobFactory: jobFactory,
	}
}

func (f *runnerFactory) Make(jobType, jobName string, jobBytes []byte, requestId uint) (Runner, error) {
	// Instantiate a "blank" job of the given type
	job, err := f.jobFactory.Make(jobType, jobName)
	if err != nil {
		return nil, err
	}

	// Have the job re-create itself so it's no longer blank but rather
	// what it was when first created in the Request Manager
	if err := job.Deserialize(jobBytes); err != nil {
		return nil, err
	}

	// Job should be ready to run. Create and return a runner for it.
	return NewJobRunner(job, requestId), nil
}
