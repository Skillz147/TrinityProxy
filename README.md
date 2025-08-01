# TrinityProxy
### Enterprise-Grade SOCKS5 Proxy Network Management System

[![Go Version](https://img.shields.io/badge/Go-1.24.3-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Build Status](https://img.shields.io/badge/Build-Passing-brightgreen.svg)](Makefile)

TrinityProxy is a sophisticated, distributed SOCKS5 proxy network management system designed for enterprise-scale deployments. It provides centralized control, automated deployment, health monitoring, and geographic routing capabilities for managing multiple SOCKS5 proxy servers across different VPS instances.

## 🏗️ Architecture Overview

TrinityProxy operates on a **Controller-Agent** architecture:

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Controller    │    │    Agent VPS    │    │    Agent VPS    │
│  (API Server)   │◄──►│  SOCKS5 Proxy   │    │  SOCKS5 Proxy   │
│                 │    │   + Heartbeat   │    │   + Heartbeat   │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│ SQLite Database │    │ Dante Server    │    │ Dante Server    │
│  Node Registry  │    │ (port: random)  │    │ (port: random)  │
│ Health Monitor  │    │ Auth: u_xxxx    │    │ Auth: u_xxxx    │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

### Core Components

#### 1. **Controller Node** (API Server)
- **Purpose**: Central management hub for the entire proxy network
- **Responsibilities**:
  - RESTful API for proxy management
  - Node registration and health monitoring  
  - Geographic routing and load balancing
  - Database management (SQLite)
  - Real-time status reporting

#### 2. **Agent Nodes** (SOCKS5 Proxies)
- **Purpose**: Distributed proxy servers on VPS instances
- **Responsibilities**:
  - Dante SOCKS5 server installation and management
  - Heartbeat reporting to controller
  - Automatic credential generation
  - System health monitoring
  - Geographic metadata collection

#### 3. **Database Layer**
- **Technology**: SQLite with automatic schema management
- **Data Stored**:
  - Node registration details
  - Geographic information (IP geolocation)
  - Health status and uptime metrics
  - Authentication credentials
  - Performance statistics

## 🚀 Quick Start

### Prerequisites
- **Go 1.24.3+** (for building from source)
- **Linux VPS** with root access (for agents)
- **SQLite3** (automatically installed)
- **Dante SOCKS5 Server** (automatically installed)

### One-Command Setup

```bash
# Clone and setup everything
git clone https://github.com/Skillz147/TrinityProxy.git
cd TrinityProxy
make quickstart
```

This single command will:
1. ✅ Check all dependencies
2. ✅ Install Go modules
3. ✅ Build all binaries
4. ✅ Prepare the development environment

## 📋 Installation & Deployment

### Controller Setup (Management Server)

```bash
# 1. Quick setup
make quickstart

# 2. Start controller
make run-controller
# OR with environment variable
TRINITY_ROLE=controller make run
```

The controller will start an API server on port `8080` with the following endpoints:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/nodes` | GET | List all registered proxy nodes |
| `/nodes` | POST | Register a new proxy node |
| `/nodes/{id}` | GET | Get specific node details |
| `/nodes/{id}/heartbeat` | POST | Update node health status |
| `/health` | GET | Controller health check |

### Agent Setup (VPS Proxy Servers)

```bash
# 1. On each VPS, clone and setup
git clone https://github.com/Skillz147/TrinityProxy.git
cd TrinityProxy
make quickstart

# 2. Install system dependencies (requires sudo)
sudo make install

# 3. Start agent
make run-agent
# OR with environment variable  
TRINITY_ROLE=agent make run
```

The agent will:
1. 🔧 Install and configure Dante SOCKS5 server
2. 🎲 Generate random credentials and port (20000-59999)
3. 📡 Start heartbeat reporting to controller
4. 🌍 Collect geographic metadata
5. ✅ Report health status continuously

## 🎛️ Interactive Configuration

TrinityProxy features an intelligent configuration system:

```bash
# Interactive role selection
make run

# Example output:
[*] Current TRINITY_ROLE: agent
[?] Use existing role? (Y/n): n
[*] Overriding existing role...

Please select your role:
1. Controller (API Server for managing proxy nodes)
2. Agent (SOCKS5 Proxy + Heartbeat reporting)  
3. View current environment settings
4. Clear current environment settings

Enter choice (1-4): 2
```

### Environment Management Features

- **Automatic Detection**: Recognizes existing environment variables
- **Override Capability**: Always allows changing roles
- **Shell Integration**: Auto-detects bash/zsh/fish and offers persistence
- **Session Management**: Maintains settings across terminal sessions

## 🔧 Development Workflow

### Building Components

```bash
# Build everything
make build

# Individual components
make $(BUILD_DIR)/trinityproxy  # Main binary
make $(BUILD_DIR)/installer     # Agent installer
make $(BUILD_DIR)/api          # Controller API server
```

### Development with Auto-Restart

```bash
# Terminal 1: Controller with auto-restart
make dev-controller

# Terminal 2: Agent with auto-restart  
make dev-agent
```

### Code Quality

```bash
make format  # Format Go code
make lint    # Run linter (requires golangci-lint)
make test    # Run test suite
```

## 🌐 Network Operations

### Proxy Usage

Once an agent is running, you can use the SOCKS5 proxy:

```bash
# Example: Using curl through the proxy
curl --socks5 username:password@vps-ip:port http://httpbin.org/ip

# Example: Using with applications
export SOCKS_PROXY="socks5://username:password@vps-ip:port"
```

### Health Monitoring

```bash
# Check controller status
curl http://controller-ip:8080/health

# List all nodes
curl http://controller-ip:8080/nodes

# Get specific node details
curl http://controller-ip:8080/nodes/{node-id}
```

## 📊 Node Management

### Automatic Node Registration

When an agent starts, it automatically:

1. **Generates Unique Credentials**
   ```go
   username := "u_" + randomHex(4)    // e.g., u_a1b2c3d4
   password := randomHex(12)          // e.g., 1a2b3c4d5e6f7g8h9i0j1k2l
   port := random(20000, 59999)       // e.g., 45023
   ```

2. **Collects System Metadata**
   ```json
   {
     "ip": "203.0.113.1",
     "port": 45023,
     "country": "United States", 
     "city": "New York",
     "last_heartbeat": "2025-08-01T12:00:00Z",
     "status": "healthy"
   }
   ```

3. **Registers with Controller**
   - Sends heartbeat every 30 seconds
   - Reports system health metrics
   - Updates geographic information

### Geographic Routing

The controller supports filtering nodes by geographic criteria:

```bash
# Get nodes in specific country
curl "http://controller:8080/nodes?country=United%20States"

# Get nodes in specific city
curl "http://controller:8080/nodes?city=New%20York"
```

## 🔐 Security Features

### Authentication System
- **Random Credential Generation**: Each agent creates unique username/password
- **Secure Storage**: Credentials stored in `/etc/trinityproxy-*` with 600 permissions
- **No Default Passwords**: Every installation has unique authentication

### Network Security
- **Private API Communication**: Controller-agent communication on internal networks
- **Port Randomization**: SOCKS5 ports are randomly assigned (20000-59999)
- **Access Control**: Dante configuration allows controlled access patterns

### File Permissions
```bash
/etc/trinityproxy-username  # 600 (owner read/write only)
/etc/trinityproxy-password  # 600 (owner read/write only)  
/etc/trinityproxy-port     # 600 (owner read/write only)
/etc/danted.conf           # 644 (world readable, owner writable)
```

## 🛠️ Troubleshooting

### Common Issues

#### 1. **Agent Won't Start**
```bash
# Check system dependencies
make check-deps

# Install missing dependencies
sudo make install

# Check Dante service status
sudo systemctl status trinityproxy
sudo journalctl -u trinityproxy -f
```

#### 2. **Controller Connection Issues**
```bash
# Verify controller is running
curl http://controller-ip:8080/health

# Check agent heartbeat logs
tail -f /var/log/trinityproxy-agent.log
```

#### 3. **SOCKS5 Connection Fails**
```bash
# Test local SOCKS5 server
curl --socks5 127.0.0.1:$(cat /etc/trinityproxy-port) http://httpbin.org/ip

# Check Dante logs
sudo tail -f /var/log/danted.log
```

### Diagnostic Commands

```bash
# Project status
make status

# Version information  
make version

# Clean rebuild
make clean && make build

# Full system check
make check-deps
```

## 📁 Project Structure

```
TrinityProxy/
├── main.go                    # Entry point with role selection
├── go.mod                     # Go module definition
├── Makefile                   # Build and deployment automation
├── README.md                  # This documentation
│
├── cmd/                       # Executable commands
│   ├── api/
│   │   └── enhanced_main.go   # Controller API server
│   └── installer/
│       └── installer.go       # Agent SOCKS5 installer
│
├── internal/                  # Internal packages
│   ├── agent/
│   │   ├── heartbeat.go       # Heartbeat reporting system
│   │   └── identity.go        # Geographic metadata collection  
│   └── storage/
│       └── database.go        # SQLite node management
│
└── scripts/                   # Deployment scripts
    ├── setup.sh              # Basic setup script
    └── setup_api.sh          # API server setup
```

## 🔄 Deployment Scenarios

### Scenario 1: Single Controller + Multiple Agents

```bash
# Controller Server (e.g., your main server)
git clone https://github.com/Skillz147/TrinityProxy.git
cd TrinityProxy  
make quickstart
make run-controller

# Agent VPS #1 (e.g., US East Coast)
git clone https://github.com/Skillz147/TrinityProxy.git
cd TrinityProxy
make quickstart
sudo make install
CONTROLLER_URL=http://controller-ip:8080 make run-agent

# Agent VPS #2 (e.g., EU West)  
git clone https://github.com/Skillz147/TrinityProxy.git
cd TrinityProxy
make quickstart
sudo make install
CONTROLLER_URL=http://controller-ip:8080 make run-agent
```

### Scenario 2: Development Environment

```bash
# Terminal 1: Controller with auto-restart
make dev-controller

# Terminal 2: Local agent for testing
make dev-agent

# Terminal 3: Monitor logs
tail -f /var/log/trinityproxy-*.log
```

### Scenario 3: Production Deployment

```bash
# Use deployment helper
make deploy-vps VPS_HOST=root@your-vps.com

# Or manual deployment with monitoring
ssh root@vps "cd TrinityProxy && make run-agent &"
```

## 🎯 Use Cases

### 1. **Web Scraping Networks**
- Deploy agents across multiple geographic regions
- Route requests through different IP addresses
- Automatic failover when nodes go offline

### 2. **Privacy & Security**
- Personal VPN alternative with multiple exit points
- Rotating proxy endpoints for enhanced anonymity
- Geographic IP diversity for accessing region-locked content

### 3. **Load Testing & Development**
- Simulate traffic from different geographic locations
- Test applications under various network conditions
- Distributed load testing capabilities

### 4. **Enterprise Proxy Management**
- Centralized control of multiple proxy servers
- Health monitoring and automatic replacement
- Geographic routing and load balancing

## 🤝 Contributing

1. **Fork** the repository
2. **Create** a feature branch (`git checkout -b feature/amazing-feature`)
3. **Commit** your changes (`git commit -m 'Add amazing feature'`)
4. **Push** to the branch (`git push origin feature/amazing-feature`)
5. **Open** a Pull Request

### Development Guidelines

- Follow Go best practices and formatting (`make format`)
- Run tests before submitting (`make test`)
- Update documentation for new features
- Use descriptive commit messages

## 📝 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- **Dante SOCKS5 Server** - The backbone SOCKS5 implementation
- **SQLite** - Reliable embedded database
- **Go Community** - Excellent ecosystem and libraries

## 📞 Support

- **Issues**: [GitHub Issues](https://github.com/Skillz147/TrinityProxy/issues)
- **Documentation**: This README and inline code comments
- **Build System**: Run `make help` for all available commands

---

**TrinityProxy** - *Building the future of distributed proxy networks* 🚀
