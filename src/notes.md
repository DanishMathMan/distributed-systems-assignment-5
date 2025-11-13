# Replication

https://www.geeksforgeeks.org/operating-systems/what-is-replication-in-distributed-system/

Replication in distributed systems refers to the process of creating and maintaining multiple copies (replicas) of data, resources, or services across different nodes (computers or servers) within a network. The primary goal of replication is to enhance system [**reliability**](https://www.geeksforgeeks.org/system-design/reliability-in-system-design/), [**availability**](https://www.geeksforgeeks.org/system-design/availability-in-system-design/), and performance by ensuring that data or services are accessible even if some nodes fail or become unavailable.

### Importance

- **Enhanced Availability:** Replication ensures the system stays available even if some nodes fail, others take over. Users can still access data from other healthy replicas.
- **Improved Reliability:** With multiple copies of data, the system avoids single points of failure, ensuring continuous/uninterupted operation.
- **Reduced Latency:** Replicas placed closer to users reduce access time, improving speed and user experience.
- **Scalability:** Replication spreads the workload across nodes, allowing the system to handle more users or data by adding more replicas/scaling as needed.

## Implementations

## Leader-based (passive) replication

Raft is a consensus algorithm that uses leader-based replication.

One node is chosen as a leader, being the primary replica. Other nodes are backup nodes. The leader coordinates every request. This doesn’t mean every request from client has to go directly to primary. But if a backup replica receives a request, it will refer it to primary.

### Normal operation flow

1. **Request**: The front end issues the request, containing a unique identifier, to the primary replica manager.
2. **Coordination**: The primary takes each request atomically, in the order in which it receives it. It checks the unique identifier, in case it has already executed the request, and if so it simply resends the response.
3. **Execution**: The primary executes the request and stores the response.
4. **Agreement**: If the request is an update, then the primary sends the updated
state, the response and the unique identifier to all the backups. The backups send an acknowledgement.
5. **Response**: The primary responds to the front end, which hands the response back to the client.

### Why do we wait for acknowledgments?

The primary node waits for acknowledgments from all the other backups, before responding to client(s).

**What happens if one of the backup fails?**

If a backup fails, nothing happens as our replication system is fault tolerant.

**What happens if the primary replica fails?**

If a leader fails there must be some sort of consensus algorithm that runs through the backup, the system must keep running. They must reestablish the primary format by replacing with a unique backup. First they must agree what the state of the system is right, the latest update that came from the leader.

f+1 replica managers can sustain f crashes

## Leaderless (Active) replication

The system is backed up in each of the backups.

The client must be able to communicate with all of the replica nodes. Such that the client acts as the leader, when it makes a request it must wait for a reply/acknowledgment from all of the replicas.

The replica does not decide, the client must look at their responses and decide.

The node that communicates with the client becomes the leader **at that moment.**

If some of the replicas are corrupted, they go by majority vote.

**Cannot have linearizability.**

### Normal operation flow

1. **Request**:
The front adds unique identifier to the request, reliably-ordered multicasts it to the replica managers, The front end is assumed to fail by crashing at worst. It does not issue the next request until it has received a response.
2. **Coordination**:
The group communication system delivers the request to every correct replica manager in the same (total) order.
3. **Execution**:
Every replica manager executes the request deterministically.
The response contains the client’s unique request identifier.
4. **Agreement**: Unnecessary due to multicast
5. **Response**:
Each replica manager sends its response to the front end.

**This achieves sequential consistency, not linearisability**

**Assignment**

Assume only one auction running

system can make bid.

Node/process can ask server for result of auction, either tell what result of auction is if finished, can tell with timeout? or tell where currently is.

The system can tolerate one failure, if there are two failure, don’t care if system works or not. For example: bids arrives in one node but not the other.

Can create crash of server if just one node crashes, then minimum have to start with 2 nodes so the last node can continue keeping the system alive.

For communicating logic just take what we did in chat assignment, just make sure 2 nodes communicate with each other. Suggest using leader based (passive) replication.

**Report**

**Important to answer this:** Really argue if our system satisfies linearizability or sequential consistency