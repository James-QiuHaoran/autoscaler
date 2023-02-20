import random
from serverless_env import SimEnvironment
from pg import pg
from ppo import PPO
from dqn import dqn
from util import convert_state_action_to_reward
from util import convert_state_action_to_reward_overprovisioning
from util import convert_state_action_to_reward_tightpacking


def test_env(env, function_name):
    # state = env.reset(function_name)
    # print('Current state:', state)
    # env.print_info()

    # horizontal scaling tests
    test_action = {
        'vertical': 0,
        'horizontal': 1
    }
    states, _, _ = env.step(function_name, test_action)
    print('New state:', states)
    env.print_info()

    test_action = {
        'vertical': 0,
        'horizontal': -1
    }
    states, _, _ = env.step(function_name, test_action)
    print('New state:', states)
    env.print_info()

    # vertical scaling tests
    test_action = {
        'vertical': 128,
        'horizontal': 0
    }
    states, _, _ = env.step(function_name, test_action)
    print('New state:', states)
    env.print_info()

    test_action = {
        'vertical': -128,
        'horizontal': 0
    }
    states, _, _ = env.step(function_name, test_action)
    print('New state:', states)
    env.print_info()


def generate_traces(env, function_name):
    state = env.reset(function_name)

    num_steps = 10
    file = open("example-traces.csv", "w")
    file.write('avg_cpu_util,slo_preservation,total_cpu_shares,cpu_shares_others,num_containers,arrival_rate,' +
               'vertical_scaling,horizontal_action,reward\n')

    for i in range(num_steps):
        vertical_or_horizontal = random.choice([0, 1])
        vertical_action = 0
        horizontal_action = 0
        if vertical_or_horizontal == 0:
            vertical_action = random.choice([0, 128, -128])
        else:
            horizontal_action = random.choice([0, 1, -1])
        action = {
            'vertical': vertical_action,
            'horizontal': horizontal_action
        }
        next_state, reward, done = env.step(function_name, action)

        # print to file
        file.write(','.join([str(j) for j in state]) + ',' + str(vertical_action) + ',' + str(horizontal_action) +
                   ',' + str(reward) + '\n')
        state = next_state

    file.close()
    print('Trajectory generated!')


def main():
    """
    This is the main function for RL training and inference.
    """

    # create and initialize the environment for rl training
    env = SimEnvironment()
    function_name = env.get_function_name()
    print('Environment initialized for function', function_name)
    initial_state = env.reset(function_name)
    print('Initial state:', initial_state)

    # test the initialized environment
    test_env(env, function_name)
    print('')

    # print a sample trajectory
    # generate_traces(env, function_name)

    # init an RL agent
    agent_type = 'PPO'
    agent = None
    if agent_type == 'PPO':
        agent = PPO(env, function_name)
    elif agent_type == 'PG':
        agent = pg.PG(env, function_name)
    elif agent_type == 'DQN':
        agent = dqn.DQN(env, function_name)
    print('RL agent initialized!')

    # init from saved checkpoints
    use_checkpoint = False
    checkpoint_file = './checkpoints/ppo-ep0.pth.tar'
    if use_checkpoint:
        agent.load_checkpoint(checkpoint_file)

    # start RL training
    agent.train()


if __name__ == "__main__":
    main()
