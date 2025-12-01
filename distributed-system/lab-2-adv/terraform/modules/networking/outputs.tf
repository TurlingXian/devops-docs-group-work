output "main_vpc_id" {
  value = aws_vpc.main_vpc.id
}

output "public_subnets_list" {
  value = aws_subnet.public_subnets[*].id
}