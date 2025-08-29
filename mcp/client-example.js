/**
 * Example MCP Client for Colog Docker Log Server
 * Demonstrates both SSE and HTTP transport usage
 */

class CologMCPClient {
    constructor(baseUrl, apiKey = null) {
        this.baseUrl = baseUrl.replace(/\/$/, '');
        this.apiKey = apiKey;
        this.sessionId = null;
        this.eventSource = null;
        this.messageId = 0;
        this.pendingRequests = new Map();
    }

    // Initialize SSE connection
    async initSSE() {
        const url = new URL(`${this.baseUrl}/mcp`);
        if (this.sessionId) {
            url.searchParams.set('sessionId', this.sessionId);
        }
        if (this.apiKey) {
            url.searchParams.set('api_key', this.apiKey);
        }

        return new Promise((resolve, reject) => {
            this.eventSource = new EventSource(url.toString());
            
            this.eventSource.onopen = () => {
                console.log('✅ SSE connection established');
                resolve();
            };

            this.eventSource.onmessage = (event) => {
                try {
                    const data = JSON.parse(event.data);
                    this.handleMessage(data);
                } catch (error) {
                    console.error('Failed to parse SSE message:', error);
                }
            };

            this.eventSource.onerror = (error) => {
                console.error('SSE connection error:', error);
                reject(error);
            };
        });
    }

    // Handle incoming messages
    handleMessage(message) {
        if (message.id && this.pendingRequests.has(message.id)) {
            const { resolve, reject } = this.pendingRequests.get(message.id);
            this.pendingRequests.delete(message.id);
            
            if (message.error) {
                reject(new Error(message.error.message));
            } else {
                resolve(message.result);
            }
        } else if (message.method) {
            // Handle notifications
            this.handleNotification(message);
        }
    }

    // Handle server notifications
    handleNotification(notification) {
        switch (notification.method) {
            case 'capabilities':
                console.log('Server capabilities:', notification.params);
                break;
            case 'ping':
                console.log('Server ping:', new Date(notification.params.timestamp * 1000));
                break;
            default:
                console.log('Notification:', notification);
        }
    }

    // Send MCP request via HTTP
    async sendRequest(method, params = {}) {
        const id = ++this.messageId;
        const request = {
            id,
            method,
            params
        };

        const headers = {
            'Content-Type': 'application/json'
        };

        if (this.apiKey) {
            headers['X-API-Key'] = this.apiKey;
        }

        if (this.sessionId) {
            headers['X-Session-ID'] = this.sessionId;
        }

        try {
            const response = await fetch(`${this.baseUrl}/mcp`, {
                method: 'POST',
                headers,
                body: JSON.stringify(request)
            });

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }

            const result = await response.json();
            
            if (result.error) {
                throw new Error(result.error.message);
            }

            return result.result;
        } catch (error) {
            console.error('Request failed:', error);
            throw error;
        }
    }

    // Tool methods
    async listContainers(all = false) {
        return await this.sendRequest('tools/call', {
            name: 'list_containers',
            arguments: { all }
        });
    }

    async getContainerLogs(containerId, tail = 50, follow = false) {
        return await this.sendRequest('tools/call', {
            name: 'get_container_logs',
            arguments: {
                container_id: containerId,
                tail,
                follow
            }
        });
    }

    async exportLogsLLM(containerIds, format = 'markdown', tail = 100) {
        return await this.sendRequest('tools/call', {
            name: 'export_logs_llm',
            arguments: {
                container_ids: containerIds,
                format,
                tail
            }
        });
    }

    async filterContainers(filter = {}) {
        return await this.sendRequest('tools/call', {
            name: 'filter_containers',
            arguments: filter
        });
    }

    async getTools() {
        return await this.sendRequest('tools/list');
    }

    async getCapabilities() {
        const response = await fetch(`${this.baseUrl}/capabilities`);
        return await response.json();
    }

    async getHealth() {
        const response = await fetch(`${this.baseUrl}/health`);
        return await response.json();
    }

    // Close connections
    close() {
        if (this.eventSource) {
            this.eventSource.close();
            this.eventSource = null;
        }
    }
}

// Example usage
async function main() {
    // Initialize client
    const client = new CologMCPClient('http://localhost:8080', process.env.MCP_API_KEY);

    try {
        // Check server health
        console.log('🏥 Checking server health...');
        const health = await client.getHealth();
        console.log('Health:', health);

        // Get server capabilities
        console.log('🔧 Getting server capabilities...');
        const capabilities = await client.getCapabilities();
        console.log('Capabilities:', capabilities);

        // List available tools
        console.log('🛠️ Listing available tools...');
        const tools = await client.getTools();
        console.log('Tools:', tools);

        // Initialize SSE connection
        console.log('📡 Initializing SSE connection...');
        await client.initSSE();

        // List containers
        console.log('📦 Listing containers...');
        const containers = await client.listContainers();
        console.log('Containers:', containers);

        if (containers.content && containers.content[1] && containers.content[1].resource.containers.length > 0) {
            const firstContainer = containers.content[1].resource.containers[0];
            
            // Get logs from first container
            console.log(`📋 Getting logs from container: ${firstContainer.name}`);
            const logs = await client.getContainerLogs(firstContainer.id, 10);
            console.log('Logs:', logs);

            // Export logs for LLM analysis
            console.log('🤖 Exporting logs for LLM...');
            const exported = await client.exportLogsLLM([firstContainer.id], 'markdown', 20);
            console.log('Exported data size:', exported.content[1].resource.size);
        }

        // Filter nginx containers
        console.log('🔍 Filtering nginx containers...');
        const filtered = await client.filterContainers({ image: 'nginx' });
        console.log('Filtered containers:', filtered);

    } catch (error) {
        console.error('❌ Error:', error);
    } finally {
        // Clean up
        client.close();
    }
}

// Node.js specific implementation
if (typeof module !== 'undefined' && module.exports) {
    // For Node.js environment
    const fetch = require('node-fetch');
    const EventSource = require('eventsource');
    
    // Make fetch and EventSource available globally
    global.fetch = fetch;
    global.EventSource = EventSource;
    
    module.exports = { CologMCPClient };
    
    // Run example if this file is executed directly
    if (require.main === module) {
        main().catch(console.error);
    }
}

// Browser specific implementation
if (typeof window !== 'undefined') {
    window.CologMCPClient = CologMCPClient;
    
    // Example usage in browser
    window.runCologExample = main;
}

// Example configurations for different MCP clients

// Claude Desktop Configuration
const claudeDesktopConfig = {
    "mcpServers": {
        "colog": {
            "command": "docker",
            "args": [
                "run", "-i", "--rm",
                "-v", "/var/run/docker.sock:/var/run/docker.sock",
                "berkantay/colog-mcp:latest"
            ],
            "env": {
                "MCP_PORT": "8080",
                "MCP_HOST": "0.0.0.0"
            }
        }
    }
};

// Remote SSE Configuration
const remoteSSEConfig = {
    "mcpServers": {
        "colog-remote": {
            "command": "npx",
            "args": ["-y", "@modelcontextprotocol/server-everything"],
            "env": {
                "MCP_SERVER_URL": "http://your-server.com:8080/mcp",
                "MCP_API_KEY": "your-api-key-here"
            }
        }
    }
};

console.log('Colog MCP Client loaded. Example configurations:');
console.log('Claude Desktop:', JSON.stringify(claudeDesktopConfig, null, 2));
console.log('Remote SSE:', JSON.stringify(remoteSSEConfig, null, 2));