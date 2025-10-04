terraform {
  backend "s3" {
    bucket         = "gogs-terraform"
    key            = "eks/prod/terraform.tfstate"
    region         = "us-east-1"
    encrypt        = true
    dynamodb_table = "terraform-locks"
  }
}

