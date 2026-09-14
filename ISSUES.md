# Current Known Issues

* When a state change in the mesh occurs on a leaf node, our UI does not get a refresh ( we only refresh if peer node impacts us ). We need to propagate the leaf node change to all other nodes.
* Stats updates require a manual refresh
