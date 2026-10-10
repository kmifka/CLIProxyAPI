package pluginhost

// requiredSchedulerError is created only at the required scheduler boundary;
// general RPC execution errors deliberately do not expose ErrorClass.
type requiredSchedulerError struct {
	cause error
	class string
}

func (e requiredSchedulerError) Error() string      { return e.cause.Error() }
func (e requiredSchedulerError) Unwrap() error      { return e.cause }
func (e requiredSchedulerError) ErrorClass() string { return e.class }
