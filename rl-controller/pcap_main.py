import argparse

from pcap_env import PCAPEnvironment
from ppo import PPO


def main():
    """
    This is the main function for RL training and inference.
    """
    parser = argparse.ArgumentParser()
    parser.add_argument('--app_name', type=str, default='pcap-controller')  # name of the app to control
    parser.add_argument('--app_namespace', type=str, default='edge-system-health-pcap')  # namespace of the app
    parser.add_argument('--mpa_name', type=str, default='pcap-controller-mpa')  # name of the mpa object
    parser.add_argument('--mpa_namespace', type=str, default='edge-system-health-pcap')  # namespace of the mpa
    options = parser.parse_args()

    # create and initialize the environment for rl training
    print('Initializing environment for app', options.app_name, '('+options.app_namespace+')')
    env = PCAPEnvironment(app_name=options.app_name, app_namespace=options.app_namespace, mpa_name=options.mpa_name, mpa_namespace=options.mpa_namespace)
    print('Initial state:')
    env.print_info()

    # init an RL agent
    agent = PPO(env)
    print('RL agent initialized!')

    # init from saved checkpoints
    use_checkpoint = False
    if use_checkpoint:
        checkpoint_file = './checkpoints/ppo-ep0.pth.tar'
        agent.load_checkpoint(checkpoint_file)

    # start RL training
    agent.train()


if __name__ == "__main__":
    main()
