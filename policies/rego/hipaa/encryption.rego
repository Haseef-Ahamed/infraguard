package infraguard.hipaa.encryption

import rego.v1

deny contains msg if {
    input.resource_type == "aws_security_group"
    rule := input.ingress_rules[_]
    rule.from_port <= 5432
    rule.to_port >= 5432
    rule.cidrs[_] == "0.0.0.0/0"
    msg := {
        "rule_id": "HIPAA_164.312e2ii",
        "framework": "HIPAA",
        "severity": "CRITICAL",
        "description": "Database port exposed to internet — PHI transmission security violated",
        "remediation": "Restrict database access; enable encryption in transit",
        "resource_id": input.resource_id,
    }
}