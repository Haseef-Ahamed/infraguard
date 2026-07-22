package infraguard.cis.network_test

import rego.v1
import data.infraguard.cis.network

test_db_port_exposed if {
    count(network.deny) == 1 with input as {
        "resource_type": "aws_security_group",
        "resource_id": "sg-test01",
        "ingress_rules": [{"from_port": 5432, "to_port": 5432, "cidrs": ["0.0.0.0/0"]}]
    }
}

test_https_allowed if {
    count(network.deny) == 0 with input as {
        "resource_type": "aws_security_group",
        "resource_id": "sg-test02",
        "ingress_rules": [{"from_port": 443, "to_port": 443, "cidrs": ["0.0.0.0/0"]}]
    }
}

test_ssh_restricted_cidr if {
    count(network.deny) == 0 with input as {
        "resource_type": "aws_security_group",
        "resource_id": "sg-test03",
        "ingress_rules": [{"from_port": 22, "to_port": 22, "cidrs": ["10.0.0.0/8"]}]
    }
}