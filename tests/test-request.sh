curl -s  http://localhost:10080/api/ps  -H "Content-Type: application/json" | jq .

curl  http://localhost:10080/api/chat \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3.8:latest",
    "stream": false,
    "messages": [
      {
        "role": "user",
        "content": "Hi Friend!"
      }
    ]
  }' 


curl  http://10.0.0.26:11434/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gemma4:31b","messages":[{"role":"user","content":"hello?"},{"role":"user","content":"hi"},{"role":"user","content":"hi"},{"role":"user","content":"hi"}],"stream":true}'