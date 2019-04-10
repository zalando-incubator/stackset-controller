# canary: Automated Gradual Deployments with Feedback Loop

The `canary` package implements "automated gradual deployments with feedback loop"
(or just "gradual deployments" for short) in the stackset controller.
This document summarizes what gradual deployments are and how to use them.

Gradual deployments are driven by annotations on the StackSet and on the Stack being deployed.
In the following description, all annotations should be prefixed with `stackset-controller.zalando.org/canary/`
(omitted for readability).

Gradual deployments work as follows:

1. A StackSet is created with the `gradual-deployments-enabled` annotation.
   Under the hood, this makes it use a `canary.TrafficReconciler` (CTR) instead of the usual
   `SimpleTrafficReconciler` or `PrescaleTrafficReconciler`.
   The CTR wraps one of these to delegate usual stackset manipulations (`ReconcileDeployments`, etc.).
   
1. A Stack is created with the `is-green` annotation, as well as a few mandatory metadata annotations
   (see `controller/canary/annotations.go`).
   It is a "green stack" (by virtue of `is-green`), a concept that we explain below.
   
   The **`blue` annotation** holds the name of the "blue stack", which is another, older Stack from which
   all traffic will be transferred to the green stack in a progressive ("gradual") way.
   The goal of gradual deployments is to continuously monitor the performance of the green stack
   and compare it to the performance of the blue stack, so that we can rollback as soon as possible
   to the (stable) blue stack in case of a regression in the green stack.
   
   However, we do not directly compare blue and green stacks, because the blue stack is older and "warmer".
   Instead, we introduce a third stack, "baseline", that runs blue code but in the same context (age, replicas)
   as green. The baseline stack name is held by the **`baseline` annotation**.
   
   The **`desired-switch-duration` annotation** determines how long the progressive traffic switch will take.
   The stackset controller will adapt the size of each traffic switch increment to approximately match that
   duration.

1. Green stacks (stacks with the `is-green` annotation) are analyzed in the new `ReconcileStacks` method
   of the TrafficReconciler, which decides the direction of traffic switch (continue deploying, or rollback),
   creates the baseline stack when needed, and updates the stack annotations.
   
1. `ReconcileIngress` computes the new traffic values according to `ReconcileStacks`' decision, and calls the
   underlying `TrafficReconciler.ReconcileIngress` method to abstract away pre-scaling details.

1. After the green stack reaches 100% of the target traffic (both blue and baseline are at 0%), its performance
   is still monitored according to the `desired-final-observation-duration` annotation.
   
1. After the observation period, the `is-green` annotation is removed and the stack is treated just like
   any other. Blue and baseline are not specially removed: they will be deleted according to the stack limit.
