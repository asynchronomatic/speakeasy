curl -s  http://localhost:4080/v1/models  -H "Content-Type: application/json" \
  -H 'Authorization: Bearer <token>' | jq .

curl  http://localhost:4080/api/chat \
  -H "Content-Type: application/json" \
  -H 'Authorization: Bearer <token>' \
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

curl  http://localhost:4080/av1/chat/completions \
  -H "Content-Type: application/json" \
  -H 'Authorization: Bearer <token>' \
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
