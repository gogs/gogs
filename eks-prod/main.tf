data "aws_availability_zones" "available" {}

module "vpc" {
  source  = "./modules/vpc"
  region  = var.region
  cluster_name = var.cluster_name
}

module "iam" {
  source        = "./modules/iam"
  cluster_name  = var.cluster_name
}

module "eks" {
  source        = "./modules/eks"
  cluster_name  = var.cluster_name
  vpc_id        = module.vpc.vpc_id
  private_subnets = module.vpc.private_subnets
  node_role_arn = module.iam.node_role_arn
  region        = var.region
}

