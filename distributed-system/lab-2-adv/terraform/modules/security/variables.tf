variable "res_prefix" {
  description = "The prefix in resource's name to indicate environment and region"
  type        = string
  default     = "due1"
}

variable "main_vpc_id" {
  description = "The ID of main VPC"
  type        = string
  default     = null
}