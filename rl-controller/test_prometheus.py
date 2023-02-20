import numpy as np
from kubernetes import client, config
from prometheus_adaptor import *


# PROM_URL = 'https://thanos-querier-openshift-monitoring.apps.ci-ln-xxxxx-xxxxx.aws-2.ci.openshift.org'
# PROM_URL = 'https://prometheus-k8s.openshift-monitoring.svc.cluster.local:9091'
PROM_URL = 'https://prometheus-k8s-openshift-monitoring.edge-infra-9ca4d14d48413d18ce61b80811ba4308-0000.us-south.containers.appdomain.cloud'
# PROM_URL = 'http://localhost:9090'
# PROM_TOKEN = 'eyJhbGcxxx'
PROM_TOKEN = 'sha256~YpkDfTE_Hug1xkkK-M1rLDIRXkewYLSed-QlgGnz6x4'
FORECASTING_SIGHT_SEC = 300

def get_target_containers(corev1_client, target_namespace, target_ref):
    target_pods = corev1_client.list_namespaced_pod(namespace=target_namespace, label_selector="app=" + target_ref["name"])

    # Retrieve the target containers
    target_containers = []
    for pod in target_pods.items:
        for container in pod.spec.containers:
            if container.name not in target_containers:
                target_containers.append(container.name)

    return target_containers

def test(corev1, prom_client):
    # example target_ref {'apiVersion': 'apps/v1', 'kind': 'Deployment', 'name': 'hamster'}
    # target_ref = {'apiVersion': 'apps/v1', 'kind': 'Deployment', 'name': 'hamster'}
    target_ref = {'apiVersion': 'apps/v1', 'kind': 'Deployment', 'name': 'pcap-controller', 'namespace': 'edge-system-health-pcap'}
    print('Target Ref:', target_ref)

    # Retrieve the target pods
    if "namespace" in target_ref.keys():
        target_namespace = target_ref["namespace"]
    else:
        target_namespace = 'default'

    # Build the prometheus query for the target resources of target containers in target pods
    namespace_query = "namespace=\'" + target_namespace + "\'"

    # Get the target containers
    target_containers = get_target_containers(corev1, target_namespace, target_ref)
    print('Target containers:', target_containers)

    # Get the target container traces
    traces = {}

    container_queries = []
    for container in target_containers:
        container_query = "container='" + container + "'"
        container_queries.append(container_query)

        prom_client.update_period(FORECASTING_SIGHT_SEC)
        controlled_resources = ['cpu', 'memory']
        for resource in controlled_resources:
            if resource.lower() == "cpu":
                resource_query = "rate(container_cpu_usage_seconds_total{%s}[1m])"
            elif resource.lower() == "memory":
                resource_query = "container_memory_usage_bytes{%s}"
            elif resource.lower() == "blkio":
                resource_query = "container_fs_usage_bytes{%s}"
            elif resource.lower() == "network":
                resource_query = "rate(container_network_receive_bytes_total{%s}[1m])"
            else:
                print("Unsupported resource: " + resource)
                break

            # Retrieve the metrics for target containers in all pods
            for container_query in container_queries:
                # Retrieve the metrics for the target container
                query_index = namespace_query + "," + container_query

                query = resource_query % (query_index)
                print(query)

                # Retrieve the metrics for the target container
                traces = prom_client.get_promdata(query, traces, resource)

    # custom metrics exported to prometheus
    custom_metrics = ['event_pcap_file_discovery_rate', 'event_pcap_rate_processing', 'event_pcap_rate_ingestion', 'event_pcap_rate']
    for metric in custom_metrics:
        metric_query = metric.lower()
        print(metric_query)

        # Retrieve the metrics
        traces = prom_client.get_promdata(metric_query, traces, metric)
    # print('Collected Traces:', traces)
    print('Collected traces for', target_ref['name'])
    cpu_traces = traces[target_ref['name']]['cpu']
    memory_traces = traces[target_ref['name']]['memory']
    # blkio_traces = traces[target_ref['name']]['blkio']

    # Compute the average utilizations
    for container in cpu_traces:
        cpu_utils = []
        for measurement in cpu_traces[container]:
            cpu_utils.append(float(measurement[1]))
        print('Avg CPU Util ('+container+'):', np.mean(cpu_utils))
    for container in memory_traces:
        memory_usages = []
        for measurement in memory_traces[container]:
            memory_usages.append(int(measurement[1]) / 1024 / 1024.0)
        print('Avg Memory Usage ('+container+'):', np.mean(memory_usages), 'MB')

    for metric in custom_metrics:
        if metric.lower() == 'event_pcap_file_discovery_rate':
            metric_traces = traces['pcap-scheduler'][metric.lower()]
            rate = []
            for container in metric_traces:
                values = []
                for measurement in metric_traces[container]:
                    values.append(float(measurement[1]))
                print(container, np.mean(values))
                rate.append(np.mean(values))
            print('Avg PCAP file discovery rate:', np.mean(rate))
            print('Total PCAP file discovery rate:', sum(rate))
        elif metric.lower() == 'event_pcap_rate_processing':
            metric_traces = traces['pcap-log-monitor'][metric.lower()]
            rate = []
            for container in metric_traces:
                values = []
                for measurement in metric_traces[container]:
                    values.append(float(measurement[1]))
                print(container, np.mean(values))
                rate.append(np.mean(values))
            print('Avg PCAP processing rate:', np.mean(rate))
            print('Total PCAP processing rate:', sum(rate))
        elif metric.lower() == 'event_pcap_rate_ingestion':
            metric_traces = traces['pcap-log-monitor'][metric.lower()]
            rate = []
            for container in metric_traces:
                values = []
                for measurement in metric_traces[container]:
                    values.append(float(measurement[1]))
                print(container, np.mean(values))
                rate.append(np.mean(values))
            print('Avg PCAP ingestion rate:', np.mean(rate))
            print('Total PCAP ingestion rate:', sum(rate))
        elif metric.lower() == 'event_pcap_rate':
            metric_traces = traces['pcap-log-monitor'][metric.lower()]
            rate = []
            for container in metric_traces:
                values = []
                for measurement in metric_traces[container]:
                    values.append(float(measurement[1]))
                print(container, np.mean(values))
                rate.append(np.mean(values))
            print('Avg PCAP rate:', np.mean(rate))
            print('Total PCAP rate:', sum(rate))

def main():
    # Load cluster config
    if 'KUBERNETES_PORT' in os.environ:
        config.load_incluster_config()
    else:
        config.load_kube_config()

    # Get the api instance to interact with the cluster
    api_client = client.api_client.ApiClient()
    corev1 = client.CoreV1Api(api_client)

    # Create a Prometheus client
    prom_client = PromCrawler(prom_address=PROM_URL, prom_token=PROM_TOKEN)

    test(corev1, prom_client)


if __name__ == "__main__":
    main()
