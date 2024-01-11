package core

var (
	SyncIngressAnnotationKeys = []string{
		"alb.ingress.kubernetes.io/ip-address-type",
		"zalando.org/aws-load-balancer-ssl-cert",
		"zalando.org/aws-load-balancer-scheme",
		"zalando.org/aws-load-balancer-shared",
		"zalando.org/aws-load-balancer-security-group",
		"zalando.org/aws-load-balancer-ssl-policy",
		"zalando.org/aws-load-balancer-type",
		"zalando.org/aws-load-balancer-http2",
		"zalando.org/aws-waf-web-acl-id",
		"kubernetes.io/ingress.class",
	}
)
