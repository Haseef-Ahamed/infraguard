variable "environment" { default = "dev" }

resource "aws_s3_bucket" "artifacts" {
  bucket = "infraguard-artifacts-${var.environment}"
  tags = {
    Name        = "infraguard-artifacts"
    Environment = var.environment
    ManagedBy   = "opentofu"
  }
}

resource "aws_s3_bucket_public_access_block" "artifacts" {
  bucket                  = aws_s3_bucket.artifacts.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_versioning" "artifacts" {
  bucket = aws_s3_bucket.artifacts.id
  versioning_configuration { status = "Enabled" }
}

output "bucket_name" { value = aws_s3_bucket.artifacts.bucket }
output "bucket_arn" { value = aws_s3_bucket.artifacts.arn }
