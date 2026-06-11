terraform {
  required_version = ">= 1.7.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  access_key = var.aws_access_key
  secret_key = var.aws_secret_key
  region     = var.aws_region

  endpoints {
    ec2 = var.aws_endpoint
    s3  = var.aws_endpoint
    iam = var.aws_endpoint
  }

  skip_credentials_validation = true
  skip_requesting_account_id  = true
  skip_metadata_api_check     = true
  s3_use_path_style           = true
}

module "vpc" {
  source      = "./aws/vpc"
  environment = var.environment
}

module "security_groups" {
  source      = "./aws/security_groups"
  vpc_id      = module.vpc.vpc_id
  environment = var.environment
}

module "s3" {
  source      = "./aws/s3"
  environment = var.environment
}

output "vpc_id"            { value = module.vpc.vpc_id }
output "security_group_id" { value = module.security_groups.security_group_id }
output "s3_bucket"         { value = module.s3.bucket_name }
