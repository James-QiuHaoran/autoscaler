import os
import time
from util import *
from kubernetes import client, config
from kubernetes.client.rest import ApiException
from prometheus_adaptor import *


MPA_DOMAIN = 'autoscaling.k8s.io'
MPA_VERSION = 'v1alpha1'
MPA_PLURAL = 'multidimpodautoscalers'
PROM_URL = 'https://prometheus-k8s-openshift-monitoring.edge-infra-9ca4d14d48413d18ce61b80811ba4308-0000.us-south.containers.appdomain.cloud'
PROM_TOKEN = None  # 'sha256~ffmVh28IPNrt6Ps0FF79Zh9S08kmXm2ay4gicSEguN8'
FORECASTING_SIGHT_SEC = 30  # look back for 30s in Prometheus
HORIZONTAL_SCALING_INTERVAL = 10  # wait for 10s for horizontal scaling to update
VERTICAL_SCALING_INTERVAL = 10    # wait for 10s for vertical scaling to update

class PCAPEnvironment:
    app_name = 'pcap-controller'
    app_namespace = 'edge-system-health-pcap'
    mpa_name = 'pcap-controller-mpa'
    mpa_namespace = 'edge-system-health-pcap'

    # initial resource allocations
    initial_pcap_controllers = 1
    initial_tek_controllers = 2
    initial_cpu_limit = 1024      # millicore
    initial_memory_limit = 2048   # MiB

    # states
    controlled_resources = ['cpu', 'memory', 'blkio']
    custom_metrics = [
        'event_pcap_file_discovery_rate',
        'event_pcap_rate_processing', 'event_pcap_rate_ingestion', 'event_pcap_rate'  # ,
        # 'event_tek_rate_processing', 'event_tek_rate_ingestion', 'event_tek_rate'
    ]
    states = {
        # system-wise metrics
        'cpu_util': 0.0,                   # 0
        'memory_util': 0.0,                # 1
        'disk_io_usage': 0.0,              # 2
        # 'ingress_rate': 0.0,
        # 'egress_rate': 0.0,
        # application-wise metrics
        'pcap_file_discovery_rate': 0.0,   # 3
        'pcap_rate': 0.0,  # 1 / lag       # 4
        'pcap_processing_rate': 0.0,       # 5
        'pcap_ingestion_rate': 0.0,        # 6
        'tek_rate': 0.0,  # 1 / lag        # 7
        'tek_processing_rate': 0.0,        # 8
        'tek_ingestion_rate': 0.0,         # 9
        # resource allocation
        # 'num_pcap_schedulers': 1,
        # 'num_pcap_controllers': 1,
        # 'num_tek_controllers': 2,
        'num_replicas': 1,                 # 10
        'cpu_limit': 1024,                 # 11
        'memory_limit': 2048,              # 12
    }

    # action and reward at the previous time step
    last_action = {
        'vertical_cpu': 0,
        'vertical_memory': 0,
        'horizontal': 0
    }
    last_reward = 0

    def __init__(self, app_name='pcap-controller', app_namespace='edge-system-health-pcap', mpa_name='pcap-controller-mpa', mpa_namespace='edge-system-health-pcap'):
        if app_name not in ['pcap-controller', 'tek-controller', 'hamster']:
            print('Application not recognized! Please choose from the following [pcap-controller, tek-controller].')
        self.app_name = app_name
        self.app_namespace = app_namespace
        self.mpa_name = mpa_name
        self.mpa_namespace = mpa_namespace

        # load cluster config
        if 'KUBERNETES_PORT' in os.environ:
            config.load_incluster_config()
        else:
            config.load_kube_config()

        # get the api instance to interact with the cluster
        api_client = client.api_client.ApiClient()
        self.api_instance = client.AppsV1Api(api_client)
        self.corev1 = client.CoreV1Api(api_client)

        # set up the prometheus client
        if not os.getenv("PROM_TOKEN"):
            print("PROM_TOKEN not set!")
            exit()
        else:
            PROM_TOKEN = os.getenv("PROM_TOKEN")
        self.prom_client = PromCrawler(prom_address=PROM_URL, prom_token=PROM_TOKEN)

        # current resource limit
        self.states['cpu_limit'] = self.initial_cpu_limit
        self.states['memory_limit'] = self.initial_memory_limit
        self.states['num_replicas'] = 2
        if self.app_name == 'pcap-controller':
            self.states['num_replicas'] = self.initial_pcap_controllers
        elif self.app_name == 'tek-controller':
            self.states['num_replicas'] = self.initial_tek_controllers

        # self.observe_states()
        self.init()

    # observe the current states
    def observe_states(self):
        target_containers = self.get_target_containers()
        print('target_containers:', target_containers)
        # get system metrics for target containers
        traces = {}
        namespace_query = "namespace=\'" + self.app_namespace + "\'"
        container_queries = []
        self.prom_client.update_period(FORECASTING_SIGHT_SEC)
        for container in target_containers:
            container_query = "container='" + container + "'"
            container_queries.append(container_query)

            for resource in self.controlled_resources:
                if resource.lower() == "cpu":
                    resource_query = "rate(container_cpu_usage_seconds_total{%s}[1m])"
                elif resource.lower() == "memory":
                    resource_query = "container_memory_usage_bytes{%s}"
                elif resource.lower() == "blkio":
                    resource_query = "container_fs_usage_bytes{%s}"
                elif resource.lower() == "ingress":
                    resource_query = "rate(container_network_receive_bytes_total{%s}[1m])"
                elif resource.lower() == "egress":
                    resource_query = "rate(container_network_transmit_bytes_total{%s}[1m])"

                # retrieve the metrics for target containers in all pods
                for container_query in container_queries:
                    query_index = namespace_query + "," + container_query
                    query = resource_query % (query_index)
                    print(query)

                    # retrieve the metrics for the target container from Prometheus
                    traces = self.prom_client.get_promdata(query, traces, resource)

        # custom metrics exported to prometheus
        for metric in self.custom_metrics:
            metric_query = metric.lower()
            print(metric_query)

            # retrieve the metrics from Prometheus
            traces = self.prom_client.get_promdata(metric_query, traces, metric)

        # print('Collected Traces:', traces)
        print('Collected traces for', self.app_name)
        cpu_traces = traces[self.app_name]['cpu']
        memory_traces = traces[self.app_name]['memory']
        blkio_traces = traces[self.app_name]['blkio']
        # ingress_traces = traces[self.app_name]['ingress']
        # egress_traces = traces[self.app_name]['egress']

        # compute the average utilizations
        if 'cpu' in self.controlled_resources:
            all_values = []
            for container in cpu_traces:
                cpu_utils = []
                for measurement in cpu_traces[container]:
                    cpu_utils.append(float(measurement[1]))
                print('Avg CPU Util ('+container+'):', np.mean(cpu_utils))
                all_values.append(np.mean(cpu_utils))
            self.states['cpu_util'] = np.mean(all_values)
        if 'memory' in self.controlled_resources:
            all_values = []
            for container in memory_traces:
                memory_usages = []
                for measurement in memory_traces[container]:
                    memory_usages.append(int(measurement[1]) / 1024 / 1024.0)
                print('Avg Memory Usage ('+container+'):', np.mean(memory_usages), 'MiB', '| Limit:', self.states['memory_limit'], 'MiB')
                all_values.append(np.mean(memory_usages))
            self.states['memory_util'] = np.mean(all_values) / self.states['memory_limit']
        if 'blkio' in self.controlled_resources:
            all_values = []
            for container in blkio_traces:
                blkio_usages = []
                for measurement in blkio_traces[container]:
                    blkio_usages.append(int(measurement[1]) / 1024 / 1024.0)
                print('Avg Disk I/O Usage ('+container+'):', np.mean(blkio_usages), 'MiB')
                all_values.append(np.mean(blkio_usages))
            self.states['disk_io_usage'] = np.mean(all_values)
        if 'ingress' in self.controlled_resources:
            all_values = []
            for container in ingress_traces:
                ingress = []
                for measurement in ingress_traces[container]:
                    ingress.append(int(measurement[1]) / 1024.0)
                print('Avg Ingress ('+container+'):', np.mean(ingress), 'KiB/s')
                all_values.append(np.mean(ingress))
            self.states['ingress_rate'] = np.mean(all_values)
        if 'egress' in self.controlled_resources:
            all_values = []
            for container in egress_traces:
                egress = []
                for measurement in egress_traces[container]:
                    egress.append(int(measurement[1]) / 1024.0)
                print('Avg egress ('+container+'):', np.mean(egress), 'KiB/s')
                all_values.append(np.mean(egress))
            self.states['egress_rate'] = np.mean(all_values)

        # get the custom metrics (PCAP-related)
        if 'event_pcap_file_discovery_rate' in self.custom_metrics:
            if 'pcap-scheduler' not in traces:
                print('Metric event_pcap_file_discovery_rate not found!')
            elif 'event_pcap_file_discovery_rate' not in traces['pcap-scheduler']:
                print('Metric event_pcap_file_discovery_rate not found!')
            else:
                metric_traces = traces['pcap-scheduler']['event_pcap_file_discovery_rate']
                rate = []
                for trace in metric_traces:
                    values = []
                    for measurement in metric_traces[trace]:
                        values.append(float(measurement[1]))
                    rate.append(np.mean(values))
                print('Avg PCAP file discovery rate:', np.mean(rate))
                print('Total PCAP file discovery rate:', sum(rate))
                self.states['pcap_file_discovery_rate'] = np.mean(rate)
        if 'event_pcap_rate_processing' in self.custom_metrics:
            if 'pcap-log-monitor' not in traces:
                print('Metric event_pcap_rate_processing not found!')
            elif 'event_pcap_rate_processing' not in traces['pcap-log-monitor']:
                print('Metric event_pcap_rate_processing not found!')
            else:
                metric_traces = traces['pcap-log-monitor']['event_pcap_rate_processing']
                rate = []
                for container in metric_traces:
                    values = []
                    for measurement in metric_traces[container]:
                        values.append(float(measurement[1]))
                    print(container, np.mean(values))
                    rate.append(np.mean(values))
                print('Avg PCAP processing rate:', np.mean(rate))
                print('Total PCAP processing rate:', sum(rate))
                self.states['pcap_processing_rate'] = np.mean(rate)
        if 'event_pcap_rate_ingestion' in self.custom_metrics:
            if 'pcap-log-monitor' not in traces:
                print('Metric event_pcap_rate_ingestion not found!')
            elif 'event_pcap_rate_ingestion' not in traces['pcap-log-monitor']:
                print('Metric event_pcap_rate_ingestion not found!')
            else:
                metric_traces = traces['pcap-log-monitor']['event_pcap_rate_ingestion']
                rate = []
                for container in metric_traces:
                    values = []
                    for measurement in metric_traces[container]:
                        values.append(float(measurement[1]))
                    print(container, np.mean(values))
                    rate.append(np.mean(values))
                print('Avg PCAP ingestion rate:', np.mean(rate))
                print('Total PCAP ingestion rate:', sum(rate))
                self.states['pcap_ingestion_rate'] = np.mean(rate)
        if 'event_pcap_rate' in self.custom_metrics:
            if 'pcap-log-monitor' not in traces:
                print('Metric event_pcap_rate not found!')
            elif 'event_pcap_rate' not in traces['pcap-log-monitor']:
                print('Metric event_pcap_rate not found!')
            else:
                metric_traces = traces['pcap-log-monitor']['event_pcap_rate']
                rate = []
                for container in metric_traces:
                    values = []
                    for measurement in metric_traces[container]:
                        values.append(float(measurement[1]))
                    print(container, np.mean(values))
                    rate.append(np.mean(values))
                print('Avg PCAP rate:', np.mean(rate))
                print('Total PCAP rate:', sum(rate))
                self.states['pcap_rate'] = np.mean(rate)
        if 'event_tek_rate_processing' in self.custom_metrics:
            if 'tek-log-monitor' not in traces:
                print('Metric event_tek_rate_processing not found!')
            elif 'event_tek_rate_processing' not in traces['tek-log-monitor']:
                print('Metric event_tek_rate_processing not found!')
            else:
                metric_traces = traces['tek-log-monitor']['event_tek_rate_processing']
                rate = []
                for container in metric_traces:
                    values = []
                    for measurement in metric_traces[container]:
                        values.append(float(measurement[1]))
                    print(container, np.mean(values))
                    rate.append(np.mean(values))
                print('Avg TEK processing rate:', np.mean(rate))
                print('Total TEK processing rate:', sum(rate))
                self.states['tek_processing_rate'] = np.mean(rate)
        if 'event_tek_rate_ingestion' in self.custom_metrics:
            if 'tek-log-monitor' not in traces:
                print('Metric event_tek_rate_ingestion not found!')
            elif 'event_tek_rate_ingestion' not in traces['tek-log-monitor']:
                print('Metric event_tek_rate_ingestion not found!')
            else:
                metric_traces = traces['tek-log-monitor']['event_tek_rate_ingestion']
                rate = []
                for container in metric_traces:
                    values = []
                    for measurement in metric_traces[container]:
                        values.append(float(measurement[1]))
                    print(container, np.mean(values))
                    rate.append(np.mean(values))
                print('Avg TEK ingestion rate:', np.mean(rate))
                print('Total TEK ingestion rate:', sum(rate))
                self.states['tek_ingestion_rate'] = np.mean(rate)
        if 'event_tek_rate' in self.custom_metrics:
            if 'tek-log-monitor' not in traces:
                print('Metric event_tek_rate not found!')
            elif 'event_tek_rate' not in traces['tek-log-monitor']:
                print('Metric event_tek_rate not found!')
            else:
                metric_traces = traces['tek-log-monitor']['event_tek_rate']
                rate = []
                for container in metric_traces:
                    values = []
                    for measurement in metric_traces[container]:
                        values.append(float(measurement[1]))
                    print(container, np.mean(values))
                    rate.append(np.mean(values))
                print('Avg TEK rate:', np.mean(rate))
                print('Total TEK rate:', sum(rate))
                self.states['tek_rate'] = np.mean(rate)

    # initialize the environment
    def init(self):
        # rescale the number of pcap controllers and tek controllers
        if self.app_name == 'pcap-controller':
            self.api_instance.patch_namespaced_deployment_scale(
                self.app_name,
                self.app_namespace,
                {'spec': {'replicas': self.initial_pcap_controllers}}
            )
            self.states['num_replicas'] = self.initial_pcap_controllers
        elif self.app_name == 'tek-controller':
            self.api_instance.patch_namespaced_deployment_scale(
                self.app_name,
                self.app_namespace,
                {'spec': {'replicas': self.initial_tek_controllers}}
            )
            self.states['num_replicas'] = self.initial_tek_controllers
        print('Set the number of replicas to:', self.states['num_replicas'])

        # reset the cpu and memory limit
        self.states['cpu_limit'] = self.initial_cpu_limit
        self.states['memory_limit'] = self.initial_memory_limit
        self.set_vertical_scaling_recommendation(self.states['cpu_limit'], self.states['memory_limit'])
        print('Set the CPU limit to:', self.states['cpu_limit'], 'millicores, memory limit to:', self.states['memory_limit'], 'MiB')

        # get the current state
        self.observe_states()

        return self.states

    # reset the environment for RL training (not initializing the environment)
    # just observe the current state and treat it as the initial state
    def reset(self):
        # get the current state
        self.observe_states()

        return self.states

    # action sanity check
    def sanity_check(self, action):
        if action['horizontal'] != 0:
            if self.states['num_replicas'] + action['horizontal'] < MIN_INSTANCES:
                return False
            if self.states['num_replicas'] + action['horizontal'] > MAX_INSTANCES:
                return False
        elif action['vertical_cpu'] != 0:
            cpu_limit_to_set = self.states['cpu_limit'] + action['vertical_cpu']
            if cpu_limit_to_set > MAX_CPU_LIMIT or cpu_limit_to_set < MIN_INSTANCES:
                return False
            if self.states['num_replicas'] <= 1:
                # num_replicas must >= 2 when vertical scaling needs to evict the pod
                return False
        elif action['vertical_memory'] != 0:
            memory_limit_to_set = self.states['memory_limit'] + action['vertical_memory']
            if memory_limit_to_set > MAX_MEMORY_LIMIT or memory_limit_to_set < MIN_MEMORY_LIMIT:
                return False
            if self.states['num_replicas'] <= 1:
                # num_replicas must >= 2 when vertical scaling needs to evict the pod
                return False
        return True

    # get all target container names
    def get_target_containers(self):
        target_pods = self.corev1.list_namespaced_pod(namespace=self.app_namespace, label_selector="app=" + self.app_name)

        target_containers = []
        for pod in target_pods.items:
            for container in pod.spec.containers:
                if container.name not in target_containers:
                    target_containers.append(container.name)

        return target_containers

    # set the vertical scaling recommendation to MPA
    def set_vertical_scaling_recommendation(self, cpu_limit, memory_limit):
        # update the recommendations
        container_recommendation = {"containerName": "", "lowerBound": {}, "target": {}, "uncappedTarget": {}, "upperBound": {}}
        container_recommendation["lowerBound"]['cpu'] = str(cpu_limit) + 'm'
        container_recommendation["target"]['cpu'] = str(cpu_limit) + 'm'
        container_recommendation["uncappedTarget"]['cpu'] = str(cpu_limit) + 'm'
        container_recommendation["upperBound"]['cpu'] = str(cpu_limit) + 'm'
        container_recommendation["lowerBound"]['memory'] = str(memory_limit) + 'Mi'
        container_recommendation["target"]['memory'] = str(memory_limit) + 'Mi'
        container_recommendation["uncappedTarget"]['memory'] = str(memory_limit) + 'Mi'
        container_recommendation["upperBound"]['memory'] = str(memory_limit) + 'Mi'

        recommendations = []
        containers = self.get_target_containers()
        for container in containers:
            vertical_scaling_recommendation = container_recommendation.copy()
            vertical_scaling_recommendation['containerName'] = container
            recommendations.append(vertical_scaling_recommendation)

        patched_mpa = {"recommendation": {"containerRecommendations": recommendations}, "currentReplicas": self.states['num_replicas'], "desiredReplicas": self.states['num_replicas']}
        body = {"status": patched_mpa}
        mpa_api = client.CustomObjectsApi()

        # Update the MPA object
        # API call doc: https://github.com/kubernetes-client/python/blob/master/kubernetes/docs/CustomObjectsApi.md#patch_namespaced_custom_object
        try:
            mpa_updated = mpa_api.patch_namespaced_custom_object(group=MPA_DOMAIN, version=MPA_VERSION, plural=MPA_PLURAL, namespace=self.mpa_namespace, name=self.mpa_name, body=body)
            print("Successfully patched MPA object with the recommendation: %s" % mpa_updated['status']['recommendation']['containerRecommendations'])
        except ApiException as e:
            print("Exception when calling CustomObjectsApi->patch_namespaced_custom_object: %s\n" % e)

    # execute the action after sanity check
    def execute_action(self, action):
        if action['vertical_cpu'] != 0:
            # vertical scaling of cpu limit
            self.states['cpu_limit'] += action['vertical_cpu']
            self.set_vertical_scaling_recommendation(self.states['cpu_limit'], self.states['memory_limit'])
            # sleep for a period of time to wait for update
            time.sleep(VERTICAL_SCALING_INTERVAL)
        elif action['vertical_memory'] != 0:
            # vertical scaling of memory limit
            self.states['memory_limit'] += action['vertical_memory']
            self.set_vertical_scaling_recommendation(self.states['cpu_limit'], self.states['memory_limit'])
            # sleep for a period of time to wait for update
            time.sleep(VERTICAL_SCALING_INTERVAL)
        elif action['horizontal'] != 0:
            # scaling in/out
            num_replicas = self.states['num_replicas'] + action['horizontal']
            self.api_instance.patch_namespaced_deployment_scale(
                self.app_name,
                self.app_namespace,
                {'spec': {'replicas': num_replicas}}
            )
            print('Scaled to', num_replicas, 'replicas')
            self.states['num_replicas'] = num_replicas
            # sleep for a period of time to wait for update
            time.sleep(HORIZONTAL_SCALING_INTERVAL)
        else:
            # no action to perform
            print('No action')
            pass

    # RL step function to update the environment given the input actions
    # action: +/- cpu limit; +/- memory limit; +/- number of replicas
    # return: state, reward
    def step(self, action):
        curr_state = self.states.copy()

        # action correctness check:
        if not self.sanity_check(action):
            self.last_reward = -1
            return curr_state, ILLEGAL_PENALTY

        # execute the action on the cluster
        self.execute_action(action)

        # observe states
        self.observe_states()

        next_state = self.states

        # calculate the reward
        reward = convert_state_action_to_reward(next_state, action, self.last_action, curr_state, app_name=self.app_name)

        self.last_reward = reward
        self.last_action = action

        return next_state, reward

    # print state information
    def print_info(self):
        print('Application name:', self.app_name, '(namespace: ' + self.app_namespace + ')')
        print('Avg CPU utilization: {:.3f}'.format(self.states['cpu_util']))
        print('Avg memory utilization: {:.3f}'.format(self.states['memory_util']))
        print('Avg PCAP file discovery rate: {:.3f}'.format(self.states['pcap_file_discovery_rate']))
        print('Avg PCAP rate: {:.3f}'.format(self.states['pcap_rate']))
        print('Avg PCAP processing rate: {:.3f}'.format(self.states['pcap_processing_rate']))
        print('Avg PCAP ingestion rate: {:.3f}'.format(self.states['pcap_ingestion_rate']))
        print('Avg TEK rate: {:.3f}'.format(self.states['tek_rate']))
        print('Avg TEK processing rate: {:.3f}'.format(self.states['tek_processing_rate']))
        print('Avg TEK ingestion rate: {:.3f}'.format(self.states['tek_ingestion_rate']))
        # print('Num of PCAP controllers: {:d}'.format(self.states['num_pcap_controllers']))
        # print('Num of TEK controllers: {:d}'.format(self.states['num_tek_controllers']))
        # print('Num of PCAP schedulers: {:d}'.format(self.states['num_pcap_schedulers']))
        print('Num of replicas: {:d}'.format(self.states['num_replicas']))


if __name__ == '__main__':
    # testing
    env = PCAPEnvironment(app_name='pcap-controller', app_namespace='edge-system-health-pcap', mpa_name='pcap-controller-mpa', mpa_namespace='edge-system-health-pcap')
    env.print_info()
