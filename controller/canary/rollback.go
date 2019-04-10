package canary

/*
Rollbacker defines the interface to implement a strategy to rollback.

This interface is for at least two possible rollbacks in case of failures:
 - Switch all the visible traffic onto the Blue stack (full_blue_rollback.go).
 - Evenly distribute all visible traffic over the Baseline and Blue stacks.
*/
type Rollbacker interface {
	Rollback(trafficMap TrafficMap) (TrafficMap, error)
}
