#!/bin/bash
set -e

# Update system
yum update -y

# Install Docker
yum install -y docker git

# Start Docker
systemctl start docker
systemctl enable docker

# Install Docker Compose
curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
chmod +x /usr/local/bin/docker-compose

# Install ECS agent
yum install -y ecs-init

# Configure ECS agent cluster
echo "ECS_CLUSTER=${cluster_name}" >> /etc/ecs/ecs.config
echo "ECS_ENABLE_CONTAINER_METADATA=true" >> /etc/ecs/ecs.config

# Start ECS agent
systemctl start ecs
systemctl enable ecs

# Create app directory
mkdir -p /opt/raft
cd /opt/raft

# Clone or pull the latest code
git clone https://github.com/uttkarshm/raft.git . || git pull

# Build Docker image
docker build -t raft-consensus .

echo "EC2 instance setup complete"
