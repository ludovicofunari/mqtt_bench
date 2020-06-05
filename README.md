# Golang MQTT Latency Benchmark
This is a Golang benchmarking tool used to measure latency in MQTT clusters. As a starting point for the developing of 
this tool, we used [this github repository](https://github.com/hui6075/mqtt-bm-latency), from which we made some 
modifications to meet the requirements for our testing.  


```bash
$ mqtt_bench --help

  -count int
        Number of messages to send per pubclient. (default 1)
  -cv int
        Select coefficient of variation for the Lognormal distribution (default 4) (default 4)
  -dist string
        Select Poisson or Lognormal distribution (default Poisson) (default "poisson")
  -file string
        Import subscribers, publishers and topic information from file. (default "files/test_1pub.json")
  -nodeport int
        Kubernetes NodepPort for VerneMQ MQTT service. (default 30123)
  -password string
        MQTT password (empty if auth disabled)
  -pubqos int
        QoS for published messages, default is 0
  -pubrate float
        Publishing exponential rate (msg/sec). (default 1)
  -quiet
        Suppress logs while running, default is false
  -size int
        Size of the messages payload (bytes). (default 100)
  -subqos int
        QoS for subscribed messages, default is 0
  -username string
        MQTT username (empty if auth disabled)
```


This version is specific for Kubernetes deployed clusters using a _NodePort_ service to expose the MQTT cluster to 
the outside, in order to evaluate its performances, but it can be easily modified as a local deployment.

### Subscriptions and Publications
Instead of using a fixing number of MQTT clients, the tool requires a `json` file as input. This gives further
flexibility allowing a finer tuning for the measurements, for example to easily discriminate between subscribers 
and publishers for their number of connections, as well as their topics of interest.

In particular, in order to distribute the MQTT subscribers and publishers across the cluster, the input `json` file, 
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

```matlab
% returns two matrices Ns(i,k) and Np(i,k) where the element (i,k) represents the number
% of subscribers and publishers respectively, of the topic i connected to node k

% Ns(i,k) matrix where the element (i,k) represents the number of subscribers of the topic i connected to node k
% Np(i,k) matrix where the element (i,k) represents the number of publishers of the topic i connected to node k

% Ntopic: number of topics
% M: number of nodes
% st: topic index for each subscriber 
% pt: topic index for each publisher 

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

```matlab
% greedy
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

Firstly, the subscribers are spread across the cluster. 
After all the subscriptions to their designated borker are successful, the publishers can start publishing their 
messages at a specific rate, both can be fixed through the command line using the arguments `count` and `pubrate` 
as well as the size of each message using `size`. The default MQTT Quality of Service (QoS) for both subscriptions 
and publications is set to QoS 0, but it can also be changed from the command line.


The _Poisson distribution_ is the default way to publish MQTT messages, but it can be changed using the argument 
`dist` to a _Lognormal distribution_  with its _coefficient of variation_ (cv) accordingly, if burst of data are 
to be examined. 

## Running an example
An example can help to better visualize the benchmark capabilities:

```bash
./mqtt_bench -file files/social_vs_nodes_rnd_M1.json -nodeport 31947 -count 10 -pubrate 1 -quiet

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
sudo apt install