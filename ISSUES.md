# Current Known Issues

## If the admin server restarts we loose information about nodes on the network
Add code to the nodes to occasionally re-reserve with the admin server

## Stoping a node leaves their model routing information stale (add code to prune dead node code)
Detect peer loss and remove their models from the model routing table


 