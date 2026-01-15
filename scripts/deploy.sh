#!/bin/bash

# =============================================================================
# DevOps Backend - Quick Deployment Script
# =============================================================================
# This script helps you quickly set up the production environment.
# Usage: ./scripts/deploy.sh [init|start|stop|restart|logs|update]
# =============================================================================

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Functions
print_header() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}========================================${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

# Check if Docker is installed
check_docker() {
    if ! command -v docker &> /dev/null; then
        print_error "Docker is not installed. Please install Docker first."
        exit 1
    fi

    if ! command -v docker compose &> /dev/null; then
        print_error "Docker Compose is not installed. Please install Docker Compose first."
        exit 1
    fi

    print_success "Docker and Docker Compose are installed"
}

# Initialize environment
init_env() {
    print_header "Initializing Environment"

    # Create .env if not exists
    if [ ! -f .env ]; then
        print_warning ".env file not found. Creating from template..."
        cp .env.example .env
        print_success ".env file created"
        print_warning "Please edit .env file and set your configuration:"
        echo "  - GITHUB_USERNAME"
        echo "  - API keys"
        echo "  - OIDC settings"
        read -p "Press Enter to continue after editing .env..."
    else
        print_success ".env file exists"
    fi

    # Create necessary directories
    print_warning "Creating necessary directories..."
    mkdir -p data logs nginx/ssl
    chmod 755 data logs
    print_success "Directories created"

    # Login to GitHub Container Registry
    print_warning "Please login to GitHub Container Registry"
    read -p "Enter your GitHub username: " GITHUB_USER
    read -sp "Enter your GitHub Personal Access Token: " GITHUB_TOKEN
    echo ""

    echo "$GITHUB_TOKEN" | docker login ghcr.io -u "$GITHUB_USER" --password-stdin

    if [ $? -eq 0 ]; then
        print_success "Successfully logged in to GHCR"
    else
        print_error "Failed to login to GHCR"
        exit 1
    fi

    print_success "Initialization completed"
    print_warning "Next steps:"
    echo "  1. Edit configs/config.prod.yaml for your production settings"
    echo "  2. Run: ./scripts/deploy.sh start"
}

# Start services
start_services() {
    print_header "Starting Services"

    check_docker

    if [ ! -f .env ]; then
        print_error ".env file not found. Run: ./scripts/deploy.sh init"
        exit 1
    fi

    print_warning "Pulling latest images..."
    docker compose pull

    print_warning "Starting services..."
    docker compose up -d

    print_success "Services started"

    echo ""
    print_header "Service Status"
    docker compose ps

    echo ""
    print_warning "Waiting for services to be healthy..."
    sleep 5

    # Check if backend is running
    if curl -f http://localhost:52538/api/health &> /dev/null; then
        print_success "Backend is healthy"
    else
        print_warning "Backend health check failed. Check logs: ./scripts/deploy.sh logs"
    fi

    echo ""
    print_success "Deployment completed!"
    echo ""
    echo "Useful commands:"
    echo "  - View logs: ./scripts/deploy.sh logs"
    echo "  - Check status: docker compose ps"
    echo "  - Stop services: ./scripts/deploy.sh stop"
}

# Stop services
stop_services() {
    print_header "Stopping Services"
    docker compose down
    print_success "Services stopped"
}

# Restart services
restart_services() {
    print_header "Restarting Services"
    docker compose restart
    print_success "Services restarted"
}

# View logs
view_logs() {
    SERVICE=${1:-devops-backend}
    print_header "Viewing Logs for $SERVICE"
    docker compose logs -f "$SERVICE"
}

# Update services
update_services() {
    print_header "Updating Services"

    print_warning "Pulling latest images..."
    docker compose pull

    print_warning "Restarting services..."
    docker compose up -d

    print_success "Services updated"
}

# Check status
check_status() {
    print_header "Service Status"
    docker compose ps

    echo ""
    print_header "Resource Usage"
    docker stats --no-stream

    echo ""
    print_header "Recent Logs"
    docker compose logs --tail=20
}

# Backup data
backup_data() {
    print_header "Backing Up Data"

    BACKUP_DIR="./backups"
    mkdir -p "$BACKUP_DIR"

    TIMESTAMP=$(date +%Y%m%d_%H%M%S)
    BACKUP_FILE="$BACKUP_DIR/backup_$TIMESTAMP.tar.gz"

    print_warning "Creating backup..."
    docker compose exec -T devops-backend tar czf - /app/data > "$BACKUP_FILE"

    print_success "Backup created: $BACKUP_FILE"
}

# Main script
main() {
    COMMAND=${1:-help}

    case $COMMAND in
        init)
            init_env
            ;;
        start)
            start_services
            ;;
        stop)
            stop_services
            ;;
        restart)
            restart_services
            ;;
        logs)
            view_logs "${2:-devops-backend}"
            ;;
        update)
            update_services
            ;;
        status)
            check_status
            ;;
        backup)
            backup_data
            ;;
        help|*)
            echo "Usage: $0 {init|start|stop|restart|logs|update|status|backup}"
            echo ""
            echo "Commands:"
            echo "  init     - Initialize environment (first time setup)"
            echo "  start    - Start all services"
            echo "  stop     - Stop all services"
            echo "  restart  - Restart all services"
            echo "  logs     - View logs (usage: $0 logs [service-name])"
            echo "  update   - Pull latest images and restart"
            echo "  status   - Check service status and resource usage"
            echo "  backup   - Backup database"
            echo ""
            echo "Examples:"
            echo "  $0 init                    # First time setup"
            echo "  $0 start                   # Start services"
            echo "  $0 logs devops-backend     # View backend logs"
            echo "  $0 logs watchtower         # View watchtower logs"
            ;;
    esac
}

main "$@"
