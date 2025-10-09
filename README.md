# GLEC — Go Leader Election Component

GLEC is a lightweight, self-contained Raft-based leader election service written in Go. Designed for simplicity and ease of integration, GLEC allows distributed applications and microservices to reliably elect a single leader among a cluster of nodes — without the overhead of a full Raft implementation.

Whether you're building a distributed scheduler, cluster coordinator, or just need fault-tolerant leader detection, GLEC gives you the essential leader election mechanism in a minimal and modular Go package.

✨ Features

🗳️ Leader election via Raft consensus

⚙️ Lightweight

🌐 Exposes a simple JSON HTTP API for status and coordination

🔄 Automatic re-election on failure

📦 Pure Go with no external dependencies
