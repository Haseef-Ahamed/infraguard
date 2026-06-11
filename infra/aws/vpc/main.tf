variable "environment" { default = "dev" }

resource "aws_vpc" "main" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_support   = true
  enable_dns_hostnames = true
  tags = {
    Name        = "infraguard-vpc"
    Environment = var.environment
    ManagedBy   = "opentofu"
  }
}

resource "aws_subnet" "public_a" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = "10.0.1.0/24"
  availability_zone = "us-east-1a"
  tags = {
    Name        = "infraguard-public-a"
    Environment = var.environment
  }
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id
  tags   = { Name = "infraguard-igw" }
}

output "vpc_id"     { value = aws_vpc.main.id }
output "subnet_ids" { value = [aws_subnet.public_a.id] }
