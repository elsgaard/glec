# 🧭 GLEC — Go Leader Election Component

**GLEC** is a lightweight, self-contained **Raft-based leader election service** written in Go. Designed for simplicity and ease of integration, GLEC allows distributed applications and microservices to reliably elect a single leader among a cluster of nodes — without the overhead of a full Raft implementation for log replication.

Whether you're building a distributed scheduler, cluster coordinator, or just need fault-tolerant leader detection, **GLEC** provides the essential leader election mechanism in a minimal and modular Go package.

---

## ✨ Features

- 🗳️ **Leader election via Raft consensus**
- ⚙️ **Lightweight**
- 🌐 **Exposes a simple JSON HTTP API for status and coordination**
- 🔄 **Automatic re-election on failure**
- 📦 **Pure Go with no external dependencies**
- 🔍 **Easy to debug and extend**

---

## 🚀 Use Cases

- Microservices needing a single active instance (e.g. cron manager, queue processor)
- Cluster coordination or distributed lock leaders
- Learning and experimenting with Raft elections in Go

---

## 🌐 HTTP API

GLEC exposes a **minimal JSON HTTP API** for interaction, monitoring, and integration. This allows other services or monitoring systems to query the election state or verify leadership.

### Example Endpoints

```http
GET /status
```
