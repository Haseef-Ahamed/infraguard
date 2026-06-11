variable "db_username" { default = "infraguard" }
variable "db_password" { sensitive = true }
variable "environment" { default = "dev" }

resource "aws_db_instance" "postgres" {
  identifier        = "infraguard-db"
  engine            = "postgres"
  engine_version    = "16.2"
  instance_class    = "db.t3.micro"
  allocated_storage = 20
  db_name           = "infraguard"
  username          = var.db_username
  password          = var.db_password

  storage_encrypted   = true
  publicly_accessible = false
  multi_az            = false

  backup_retention_period = 7
  deletion_protection     = false
  skip_final_snapshot     = true

  tags = {
    Name        = "infraguard-db"
    Environment = var.environment
    ManagedBy   = "opentofu"
  }
}

output "rds_endpoint" { value = aws_db_instance.postgres.endpoint }
output "rds_id"       { value = aws_db_instance.postgres.id }
