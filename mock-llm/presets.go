package main

// Preset represents a predefined request/response pair for the mock LLM.
type Preset struct {
	ID              int
	Name            string
	InputTokens     int
	OutputTokens    int
	ResponseText    string
	ToolCall        *ToolCallPreset // nil if not a tool call response
	IsMultiTurn     bool
	SystemPrompt    string
	UserPrompt      string
	MultiTurnMsgs   []MultiTurnMsg
}

type ToolCallPreset struct {
	FunctionName string
	Args         map[string]any
}

type MultiTurnMsg struct {
	Role    string
	Content string
}

var Presets = []Preset{
	{
		ID:           1,
		Name:         "short_qa",
		InputTokens:  100,
		OutputTokens: 50,
		SystemPrompt: "You are a helpful assistant.",
		UserPrompt:   "What is the capital of France?",
		ResponseText: "The capital of France is Paris. It is the largest city in France and serves as the country's political, economic, and cultural center. Paris is located in the north-central part of the country along the Seine River.",
	},
	{
		ID:           2,
		Name:         "code_generation",
		InputTokens:  300,
		OutputTokens: 500,
		SystemPrompt: "You are an expert programmer. Write clean, efficient code.",
		UserPrompt:   "Write a Go function that implements a concurrent-safe LRU cache with TTL support.",
		ResponseText: `Here's a concurrent-safe LRU cache implementation in Go with TTL support:

` + "```go" + `
package cache

import (
	"container/list"
	"sync"
	"time"
)

type entry struct {
	key       string
	value     interface{}
	expiresAt time.Time
	element   *list.Element
}

type LRUCache struct {
	mu       sync.RWMutex
	capacity int
	ttl      time.Duration
	items    map[string]*entry
	order    *list.List
}

func NewLRUCache(capacity int, ttl time.Duration) *LRUCache {
	c := &LRUCache{
		capacity: capacity,
		ttl:      ttl,
		items:    make(map[string]*entry, capacity),
		order:    list.New(),
	}
	go c.cleanup()
	return c
}

func (c *LRUCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	e, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expiresAt) {
		c.mu.Lock()
		c.removeEntry(e)
		c.mu.Unlock()
		return nil, false
	}
	c.mu.Lock()
	c.order.MoveToFront(e.element)
	c.mu.Unlock()
	return e.value, true
}

func (c *LRUCache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.items[key]; ok {
		e.value = value
		e.expiresAt = time.Now().Add(c.ttl)
		c.order.MoveToFront(e.element)
		return
	}

	if c.order.Len() >= c.capacity {
		oldest := c.order.Back()
		if oldest != nil {
			c.removeEntry(oldest.Value.(*entry))
		}
	}

	e := &entry{
		key:       key,
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
	e.element = c.order.PushFront(e)
	c.items[key] = e
}

func (c *LRUCache) removeEntry(e *entry) {
	c.order.Remove(e.element)
	delete(c.items, e.key)
}

func (c *LRUCache) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for c.order.Len() > 0 {
			oldest := c.order.Back()
			e := oldest.Value.(*entry)
			if now.After(e.expiresAt) {
				c.removeEntry(e)
			} else {
				break
			}
		}
		c.mu.Unlock()
	}
}

func (c *LRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[key]; ok {
		c.removeEntry(e)
	}
}

func (c *LRUCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.order.Len()
}
` + "```" + `

This implementation provides:
- O(1) Get and Set operations using a hash map and doubly-linked list
- Thread safety via sync.RWMutex (read lock for Get, write lock for Set)
- TTL-based expiration checked on access and via background cleanup
- Automatic eviction of least recently used entries when capacity is reached

Usage example:
` + "```go" + `
cache := NewLRUCache(1000, 5*time.Minute)
cache.Set("user:123", userData)
if val, ok := cache.Get("user:123"); ok {
    fmt.Println(val)
}
` + "```",
	},
	{
		ID:           3,
		Name:         "long_text",
		InputTokens:  200,
		OutputTokens: 2000,
		SystemPrompt: "You are a knowledgeable writer. Provide detailed, comprehensive responses.",
		UserPrompt:   "Explain the history and evolution of computer networking from ARPANET to modern cloud computing.",
		ResponseText: `The history of computer networking is a fascinating journey spanning over five decades, transforming from a small experimental network into the backbone of modern civilization.

## The ARPANET Era (1969-1983)

The story begins in 1969 when the Advanced Research Projects Agency (ARPA) of the U.S. Department of Defense funded the creation of ARPANET, the first wide-area packet-switching network. The initial network connected four nodes: UCLA, Stanford Research Institute, UC Santa Barbara, and the University of Utah. The key innovation was packet switching, developed by Paul Baran and Donald Davies, which broke data into small packets that could travel independently across the network.

In 1971, Ray Tomlinson invented email, which quickly became the network's most popular application. By 1973, ARPANET had grown to 30 nodes and had made its first international connections to England and Norway. Vinton Cerf and Bob Kahn began developing TCP/IP (Transmission Control Protocol/Internet Protocol), which would become the fundamental communication protocol of the Internet.

## The Birth of the Internet (1983-1990)

On January 1, 1983, ARPANET officially adopted TCP/IP, marking what many consider the birth of the Internet. The Domain Name System (DNS) was introduced in 1984, replacing numeric addresses with human-readable names. The National Science Foundation created NSFNET in 1986, connecting five supercomputer centers and serving as a major backbone for Internet traffic.

During this period, various networking technologies emerged: Ethernet (developed by Bob Metcalfe at Xerox PARC), Token Ring (IBM), and FDDI. Local Area Networks (LANs) became common in businesses and universities, while Wide Area Networks (WANs) connected geographically dispersed locations.

## The World Wide Web Revolution (1990-2000)

Tim Berners-Lee at CERN invented the World Wide Web in 1989-1990, creating HTML, HTTP, and the first web browser. The release of the Mosaic browser in 1993 made the Web accessible to non-technical users, triggering explosive growth. The commercialization of the Internet began in earnest when NSFNET was decommissioned in 1995.

The dot-com era saw the emergence of e-commerce, search engines, and web-based services. Technologies like JavaScript, CSS, Java applets, and Flash transformed static web pages into interactive applications. Bandwidth increased dramatically with the deployment of cable modems and DSL, replacing dial-up connections.

## Broadband and Mobile Networking (2000-2010)

The early 2000s saw the widespread adoption of broadband Internet, WiFi (IEEE 802.11), and mobile data networks. The transition from 2G to 3G cellular networks enabled mobile Internet access. Web 2.0 emerged with user-generated content platforms like blogs, wikis, social networks, and video sharing sites.

Key developments included:
- Voice over IP (VoIP) disrupting traditional telephony
- Peer-to-peer networking for file sharing
- Content Delivery Networks (CDNs) for scalable content distribution
- Software-Defined Networking (SDN) concepts beginning to emerge
- IPv6 development to address IPv4 address exhaustion

## Cloud Computing and Modern Networking (2010-Present)

Amazon Web Services, launched in 2006, pioneered cloud computing, followed by Microsoft Azure and Google Cloud Platform. Cloud computing fundamentally changed how applications are deployed and managed, shifting from physical servers to virtualized, on-demand infrastructure.

Modern networking innovations include:
- 4G LTE and 5G providing gigabit mobile connectivity
- Software-Defined Networking (SDN) and Network Function Virtualization (NFV) making networks programmable
- Container networking with Docker and Kubernetes enabling microservices architectures
- Edge computing bringing processing closer to data sources
- Zero-trust security models replacing perimeter-based security
- Internet of Things (IoT) connecting billions of devices

The evolution continues with developments in quantum networking, satellite Internet constellations (like Starlink), and AI-driven network management. From four connected computers in 1969 to billions of devices today, computer networking has undergone one of the most remarkable transformations in technological history.`,
	},
	{
		ID:           4,
		Name:         "tool_call",
		InputTokens:  400,
		OutputTokens: 150,
		SystemPrompt: "You are a helpful assistant with access to tools. Use tools when appropriate.",
		UserPrompt:   "What's the current weather in Tokyo and New York?",
		ToolCall: &ToolCallPreset{
			FunctionName: "get_weather",
			Args: map[string]any{
				"locations": []string{"Tokyo", "New York"},
				"units":     "celsius",
			},
		},
		ResponseText: "I'll check the weather for both cities.",
	},
	{
		ID:          5,
		Name:        "multi_turn",
		InputTokens: 800,
		OutputTokens: 300,
		IsMultiTurn: true,
		SystemPrompt: "You are an expert data analyst. Help users understand their data.",
		MultiTurnMsgs: []MultiTurnMsg{
			{Role: "user", Content: "I have a dataset of 10,000 customer transactions. How should I start analyzing it?"},
			{Role: "model", Content: "Great question! Start with exploratory data analysis (EDA). First, examine the structure: check column types, missing values, and basic statistics using describe(). Then look at distributions of key variables like transaction amount and frequency."},
			{Role: "user", Content: "I found that 15% of records have missing values in the 'category' field. What should I do?"},
			{Role: "model", Content: "15% is significant. Before deciding, investigate if the missingness is random (MCAR), depends on other variables (MAR), or is systematic (MNAR). Check if missing categories correlate with transaction amount or date. Options include: imputation using mode or ML-based methods, creating an 'Unknown' category, or analyzing missing vs non-missing groups separately."},
			{Role: "user", Content: "The missing values seem random. I'll use mode imputation. Now I want to segment customers. What clustering approach do you recommend?"},
		},
		ResponseText: `For customer segmentation with transaction data, I recommend a multi-step approach:

**1. Feature Engineering**
Create RFM (Recency, Frequency, Monetary) features:
- Recency: days since last transaction
- Frequency: total number of transactions
- Monetary: total/average spend

Add behavioral features:
- Category diversity (number of unique categories)
- Average transaction size
- Time-between-purchases variance

**2. Clustering Method**
Start with K-Means for its simplicity and scalability with 10K records:
- Standardize features first (StandardScaler)
- Use the Elbow Method and Silhouette Score to find optimal K (typically 3-6 segments)
- Consider DBSCAN as a secondary method to detect outlier customers

**3. Validation**
- Profile each cluster with descriptive statistics
- Create visualization dashboards showing cluster distributions
- Validate business interpretability with stakeholders

Typically you'll find segments like: high-value loyalists, occasional big spenders, frequent small buyers, and at-risk churning customers.`,
	},
}
