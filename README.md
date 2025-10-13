# 🧭 GLEC — Go Leader Election Component

**GLEC** is a lightweight, self-contained **Raft-based leader election service** written in Go. Designed for simplicity and ease of integration, GLEC allows distributed applications and microservices to reliably elect a single leader among a cluster of nodes — without the overhead of a full Raft implementation.

Whether you're building a distributed scheduler, cluster coordinator, or just need fault-tolerant leader detection, **GLEC** provides the essential leader election mechanism.

---

## ✨ Features

- 🗳️ **Leader election via Raft consensus**
- ⚙️ **Lightweight and embeddable**
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
If your application or script needs to perform leader-only tasks (e.g., data sync, scheduled jobs, exclusive writes), you can use this service to check if the local instance is currently the leader.

✅ Query the /status endpoint and proceed only if the current instance role is the leader.

```http
GET /status

{
  "leader": "node-1:8000",
  "node": "node-1:8000",
  "role": "Leader",
  "term": 1
}

```

### How to run

```console
./glec -id=node-1:8000 -peers=node-2:8000,node-3:8000
```




