variable "main_region" {
  description = "The main region to provide resources or the region which management resources are placed"
  type        = string
  default     = "us-east-1"
}

variable "account_id" {
  description = "The ID of this AWS account"
  type        = string
  default     = "916343418008"
}

variable "az_list" {
  description = "The az that are associated with the region above"
  type        = list(string)
  default     = ["us-east-1a", "us-east-1b", "us-east-1c"]
}

variable "res_prefix" {
  description = "The prefix in resource's name to indicate environment and region"
  type        = string
  default     = "due1"
}

variable "main_cidr" {
  description = "Main CIDR, assigned to the management or the main VPC"
  type        = string
  default     = "10.0.0.0/16"
}

variable "private_subnets_cidr" {
  description = "CIDR for several private subnets, default is a set of 3"
  type        = list(string)
  default     = ["10.0.11.0/24", "10.0.21.0/24", "10.0.31.0/24"]
}

variable "public_subnets_cidr" {
  description = "CIDR for several public subnets, default is a set of 3"
  type        = list(string)
  default     = ["10.0.10.0/24", "10.0.20.0/24", "10.0.30.0/24"]
}

variable "default_instance_types" {
  description = "A list of instance types that can be used for computing purpose"
  type        = list(string)
  default     = ["t2.micro", "t2.small", "t2.medium"]
}