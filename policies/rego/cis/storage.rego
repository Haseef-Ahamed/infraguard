package infraguard.cis.storage

import rego.v1

deny contains msg if {
    input.resource_type == "aws_s3_bucket_public_access_block"
    input.block_public_acls == false
    msg := {
        "rule_id": "CIS_2.1.1",
        "framework": "CIS",
        "severity": "HIGH",
        "description": "S3 bucket allows public ACLs — violates CIS 2.1.1",
        "remediation": "Set block_public_acls = true",
        "resource_id": input.resource_id,
    }
}

deny contains msg if {
    input.resource_type == "aws_s3_bucket_public_access_block"
    input.block_public_policy == false
    msg := {
        "rule_id": "CIS_2.1.2",
        "framework": "CIS",
        "severity": "HIGH",
        "description": "S3 bucket policy allows public access — violates CIS 2.1.2",
        "remediation": "Set block_public_policy = true",
        "resource_id": input.resource_id,
    }
}