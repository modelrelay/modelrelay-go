# CLI example moved

The ModelRelay CLI now lives in `cmd/mr` and uses the Go SDK.

```bash
cd cmd/mr
MODELRELAY_API_KEY=mr_sk_... MODELRELAY_PROJECT_ID=... go run ./... agent test researcher --input "Analyze Q4 sales"
```
