package infraguard.soc2.access

import rego.v1

deny contains msg if {
    input.resource_type == "aws_security_group"
    rule := input.ingress_rules[_]
    rule.cidrs[_] == "0.0.0.0/0"
    rule.from_port != 443
    rule.from_port != 80
    msg := {
        "rule_id": "SOC2_CC6.1",
        "framework": "SOC2",
        "severity": "HIGH",
        "description": sprintf("Unrestricted inbound access on port %v — violates SOC2 CC6.1", [rule.from_port]),
        "remediation": "Restrict network access to authorised source IP ranges",
        "resource_id": input.resource_id,
    }
}