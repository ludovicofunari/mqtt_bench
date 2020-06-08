# Golang MQTT Latency Benchmark
This is a Golang benchmarking tool used to measure latency in MQTT clusters. As a starting point for the developing of 
this tool, we used [this GitHub repository](https://github.com/hui6075/mqtt-bm-latency), from which we made some 
modifications to meet the requirements for our testing. 

This tool requires at least a `1.14.x` version of golang to make it work.


```sh
$ mqtt_bench --help

  -count int
        Number of messages to send per pubclient (default 1)
  -cv int
        Select coefficient of variation for the Lognormal distribution (default 4).
  -dist string
        Select Poisson or Lognormal distribution (default "poisson").
  -file string
        Import subscribers, publishers and topic information from file (default "files/test_1pub.json").
  -nodeport int
        Kubernetes NodepPort for VerneMQ MQTT service (default 30123).
  -pubqos int
        QoS for published messages (default 0).
  -pubrate float
        Publishing exponential rate (msg/sec) (default 1).
  -quiet
        Suppress logs while running (default false).
  -size int
        Size of the messages payload (bytes) (default 100).
  -subqos int
        QoS for subscribed messages (default 0).
```


This version is specific for Kubernetes deployed clusters using a _NodePort_ service to expose the MQTT cluster to 
the outside, in order to evaluate its performances, but it can be easily modified as a local deployment.

### Spreading MQTT Clients Across The Cluster
Instead of using a fixing number of MQTT clients, the tool requires a `json` file as input. This gives further
flexibility allowing a finer tuning for the measurements, for example, to easily discriminate between subscribers 
and publishers for their number of connections, as well as their topics of interest.

In particular, to distribute the MQTT subscribers and publishers across the cluster, the input `json` file, 
consists of a list of publishers and subscribers. Each of them has its own unique identifier, its destination broker 
node as well as its topic list.

```json
{
    "publisher": [
        {
            "pub_id": "unique_pub_id",
            "node_id": "dst_broker_id_for_pub",
            "topic_list": [
                "topic_list_to_publish"
            ]
        }
    ],
    "subscriber": [
        {
            "sub_id": "unique_sub_id",
            "node_id": "dst_broker_id_for_sub",
            "topic_list": [
                "topic_list_to_subscribe"
            ]
        }
    ]
}
```

This file is the result of a MATLAB simulation which, depending on the algorithm, simulates which broker the MQTT 
client must be attached to, with how many topics of interest. Specifically, we used two algorithms: 
the _random-attach_ and the _greedy_ one.
Below, we can see a MATLAB code snippet of both of them.

#### Random Attach Algorithm
This algorithm simulates the role of the load-balancer that, typically, attaches the underlying TCP connection 
to one of the internal brokers, chosen at random, when an MQTT client (publisher or subscriber) 
establishes a session with the cluster. 

```matlab
% Ntopic: number of topics
% M: number of nodes
% h: user id
% k: node id
% t: topic list

for h=1:Nptot
    % for each publisher select the topic and nodes 
    k = find(Ns(pt(h),:)>0,1);
    if numel(k)==0
        k=randi(M);
    end
    Np(pt(h),k)=Np(pt(h),k)+1;

    % publisher id \t session id \t node \t topic list
    fprintf(fileIDPub,'{"pub_id" : %d.%d , "node_id" : %d , "topic_list" : [%d]}',h,1,k,h);
    if h~=Nptot
        fprintf(fileIDPub,',\n');
    end
        
end

for h=1:Nstot
    % for each subscriber select the topic and nodes 
    k = randi(M);
    t=st{h}';
    Ns(t,k)=Ns(t,k)+1;
    Sn(h)=k;  

    if length(t)>1
        fprintf(fileIDSub,'{"sub_id" : %d.%d , "node_id" : %d , "topic_list" : %s}',h,1,k,jsonencode(t));
    else
        fprintf(fileIDSub,'{"sub_id" : %d.%d , "node_id" : %d , "topic_list" : [%d]}',h,1,k,t);
    end
    if h~=Nstot
        fprintf(fileIDSub,',\n');
    end  

end
if fileIDSub~=0 
    fprintf(fileIDSub,']');
end 
```

#### Greedy Algorithm
The following greedy algorithm, aims at reducing the internal traffic.    

We define _Ta<sub>k</sub>_ as the set of _active_ topics of the _k-th_ broker, 
where a topic is active if there exists at least a subscriber or a publisher 
of that topic on the broker. Assuming that an incoming client is interested 
in a set of topics _Tc_ out of the possible _Ntop_ ones, _Tc<sub>j</sub>_ is the _j-th_ 
topic of the set _Tc_ and the size of the set is indicated as `len(Tc)`.

We assume the client is either a publisher or a subscriber.   
If the client is a subscriber, the increase of _&Delta;Ai_ of the internal traffic, resulting 
from its connection to broker _k_, can be written as the Equation 1: 

<img src="https://raw.githubusercontent.com/ludfun/mqtt_bench/master/files/eq1.png" width="250" height="100">
<img src="https://raw.githubusercontent.com/ludfun/mqtt_bench/master/files/eq2.png" width="400" height="60">  


because, if the topic _Tc<sub>j</sub>_ is not active on the _k-th_ broker (_Tc<sub>j</sub>_ &notin; _Ta<sub>k</sub>_), 
and the publisher of the topic is connected to the cluster, then the broker starts 
receiving the related publications at a rate equal to _&lambda;<sub>Tc<sub>j</sub></sub>_. Otherwise, 
no additional internal traffic is generated.

If instead the client is a publisher, the internal traffic increase _&Delta;Ai_ resulting from its connection 
to broker _k_ can be written as the Equation 3: 

<img src="https://raw.githubusercontent.com/ludfun/mqtt_bench/master/files/eq3.png" width="300" height="80">  

because, after the connection of the client, the publications of the _j-th_ topic of _Tc_ are transferred to every 
other broker having at least a subscriber, i.e. having the topic active. If the client is both a subscriber and a 
publisher we can use Eq. 1 for the set of subscribed topics and  Eq. 3 for the topics 
for which the client is a publisher.  


The Eq. 1 and 3 initially drove us to design a _best-matching_ greedy 
strategy, which follows the minimization in Eq. 4. Simply, when a new client arrives, the algorithm 
connects it to the broker that provides the lowest increase in internal traffic. However, to ensure a fair balance 
of external traffic among brokers, not all brokers can be selected. Indeed, the set of candidates is made up of 
brokers whose incoming and outgoing external traffics are equal to the fairly share values of _Aei/M_ and _Aeo/M_, 
respectively, except for a tolerance fairness factor &gamma; &ge; 1. The greater &gamma;, the smaller the fairness 
level.  

<img src="https://raw.githubusercontent.com/ludfun/mqtt_bench/master/files/eq4.png" width="180" height="120">  



```matlab
% Ntopic: number of topics
% M number of nodes
% gamma: unbalance factor
% Nses: number of session per client
% h: pub id
% j: sub id
% s: session id
% k, node_id: node id
% t: topic list

for h=1:Nptot
    % for each publisher select the topic and nodes, considering that any subscriber can be the publisher 
    k = find(Ns(pt(h),:)>0,1);
    if numel(k)==0
        k=randi(M);
    end
    Np(pt(h),k)=Np(pt(h),k)+1;

    % publisher id \t session id \t node \t topic list
    fprintf(fileIDPub,'{"pub_id" : %d.%d , "node_id" : %d , "topic_list" : [%d]}',h,1,k,h);
    if h~=Nptot
        fprintf(fileIDPub,',\n');
    end
    
end

Bs=1:M;
aCs=0;

for j=1:length(st)
    %[j M]
    Tc=st{j}';
    Tna=Tc;
    Uset=[]; % set of used brokers
    aCs=(aCs*(j-1)+setSize(length(Bs),min(Nses,length(Bs)),length(Tc)))/j; % average size of the decision space
    while length(Tna)>0
        [k cost]= best_matching_sub (Tna,Ns,Np,lambda_v,gamma,Bs,M);
        Tbest = Tna(find(cost==min(cost)));   % assign to the broker the topics with minimum cost
        Tna = setdiff(Tna,Tbest);
        Ns(Tbest,k)=Ns(Tbest,k)+1;
        Uset=union(Uset,k);
        Tcb{k,j}=union(Tcb{k,j},Tbest);
        if (length(Uset)==Nses)
            Tcb{k,j}=union(Tcb{k,j},Tna);
            Ns(Tna,k)=Ns(Tna,k)+1;
            Tna={};
        end
        Aeo=sum(lambda_v'.*Ns);
        TotAeo =  sum(Aeo); % average number of subscription per node
        TargetAeo = gamma*TotAeo/M;
        Bs=find(Aeo<TargetAeo)'; % available brokers for traffic fairness constraint
    end
    for s=1:length(Uset)
        node_id=Uset(s);

        if length(Tcb{node_id,j})>1
            fprintf(fileIDSub,'{"sub_id" : %d.%d , "node_id" : %d , "topic_list" : %s}',j,s,node_id,jsonencode(Tcb{node_id,j}));
        else
            fprintf(fileIDSub,'{"sub_id" : %d.%d , "node_id" : %d , "topic_list" : [%d]}',j,s,node_id,Tcb{node_id,j});
        end
        if j~=length(st) || s~=length(Uset)
            fprintf(fileIDSub,',\n');
        end  
    end
end
```

## Publishing
Firstly, the subscribers are spread across the cluster. 
After all the subscriptions to their designated broker are successful, the publishers can start publishing their 
messages at a specific rate, both can be fixed through the command line using the arguments `-count` and `-pubrate` 
as well as the size of each message using `-size`. The default MQTT Quality of Service (QoS) for both subscriptions 
and publications is set to QoS 0, but it can also be changed from the command line.


The _Poisson distribution_ is the default way to publish MQTT messages, but it can be changed using the argument 
`-dist` to a _Lognormal distribution_  with its _coefficient of variation_ (cv) accordingly, if a burst of data is 
to be examined. 

An example using one broker can help to better visualize the benchmark capabilities:

```sh
./mqtt_bench -file files/social_vs_nodes_rnd_M1.json -nodeport 31947 -count 10 -pubrate 1 -quiet

Published using a poisson distribution.

================= TOTAL PUBLISHER (1000) =================
Total Publish Success Ratio:   100.00% (10000/10000)
Average Runtime (sec):         10.50
Pub time min (ms):             0.01
Pub time max (ms):             25.92
Pub time mean mean (ms):       0.08
Pub time mean std (ms):        0.15
Average Bandwidth (msg/sec):   1.04
Total Bandwidth (msg/sec):     1038.83

================= TOTAL SUBSCRIBER (100) =================
Total Forward Success Ratio:      1000.00% (10000/1000)
Forward latency min (ms):         0.00
Forward latency max (ms):         84.00
Forward latency mean std (ms):    0.70
Total Mean forward latency (ms):  2.59

Total Receiving rate (msg/sec): 748.83
All jobs done. Time spent for the benchmark: 10s
======================================================

```