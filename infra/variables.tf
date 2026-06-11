variable "aws_access_key" { default = "test" }
variable "aws_secret_key" {
  default   = "test"
  sensitive = true
}
variable "aws_region"   { default = "us-east-1" }
variable "aws_endpoint" { default = "http://localhost:4566" }
variable "environment"  { default = "dev" }
variable "db_username"  { default = "infraguard" }
variable "db_password"  { sensitive = true }
