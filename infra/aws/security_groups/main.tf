variable "vpc_id" { type = string }
variable "environment" { default = "dev" }

resource "aws_security_group" "app_sg" {
  name        = "infraguard-app-sg"
  description = "Application SG — HTTPS only. Managed by OpenTofu."
  vpc_id      = var.vpc_id

  ingress {
    description = "HTTPS inbound"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    description = "All outbound"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name        = "infraguard-app-sg"
    Environment = var.environment
    ManagedBy   = "opentofu"
  }
}

output "security_group_id" { value = aws_security_group.app_sg.id }
