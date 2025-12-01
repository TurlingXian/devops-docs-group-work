locals {
  Owner = "huyhoang.ph"
  Type  = "IaC"
}

resource "aws_vpc" "main_vpc" {
  cidr_block           = var.main_cidr
  enable_dns_hostnames = true
  tags = {
    "Name"  = "${var.res_prefix}-main-vpc"
    "Owner" = local.Owner
    "Type"  = local.Type
  }
}

resource "aws_subnet" "public_subnets" {
  vpc_id            = aws_vpc.main_vpc.id
  count             = length(var.public_subnets_cidr)
  cidr_block        = element(var.public_subnets_cidr, count.index)
  availability_zone = element(var.az_list, count.index)

  tags = {
    "Name"  = "${var.res_prefix}-pub-subnet-00${count.index + 1}"
    "Owner" = local.Owner
    "Type"  = local.Type
  }
}

resource "aws_internet_gateway" "main_igw" {
  vpc_id = aws_vpc.main_vpc.id
  tags = {
    "Name"  = "${var.res_prefix}-main-igw"
    "Owner" = local.Owner
    "Type"  = local.Type
  }
}

resource "aws_route_table" "pub_route_table" {
  vpc_id = aws_vpc.main_vpc.id
  tags = {
    "Name"  = "${var.res_prefix}-pub-route-table"
    "Owner" = local.Owner
    "Type"  = local.Type
  }
}

resource "aws_route" "internet_public_route" {
  route_table_id         = aws_route_table.pub_route_table.id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.main_igw.id
}

resource "aws_route_table_association" "public_association" {
  count          = length(var.public_subnets_cidr)
  subnet_id      = element(aws_subnet.public_subnets[*].id, count.index)
  route_table_id = aws_route_table.pub_route_table.id
}