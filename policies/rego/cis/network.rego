package infraguard.cis.network

import rego.v1

# CIS 5.2 — No unrestricted SSH access
deny contains msg if {
    input.resource_type == "aws_security_group"
    rule := input.ingress_rules[_]
    rule.from_port <= 22
    rule.to_port >= 22
    rule.cidrs[_] == "0.0.0.0/0"
    msg := {
        "rule_id": "CIS_5.2",
        "framework": "CIS",
        "severity": "CRITICAL",
        "description": "Unrestricted SSH access (port 22) from 0.0.0.0/0",
        "remediation": "Remove the 0.0.0.0/0 ingress rule for port 22",
        "resource_id": input.resource_id,
    }
}

# CIS 5.4 — No unrestricted database access
deny contains msg if {
    input.resource_type == "aws_security_group"
    rule := input.ingress_rules[_]
    db_port := [3306, 5432, 1433, 27017, 6379][_]
    rule.from_port <= db_port
    rule.to_port >= db_port
    rule.cidrs[_] == "0.0.0.0/0"
    msg := {
        "rule_id": "CIS_5.4",
        "framework": "CIS",
        "severity": "CRITICAL",
        "description": sprintf("Database port %v exposed to internet — violates CIS 5.4", [db_port]),
        "remediation": "Restrict database ports to application subnet CIDR only",
        "resource_id": input.resource_id,
    }
}