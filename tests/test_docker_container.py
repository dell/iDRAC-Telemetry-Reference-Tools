import pytest
import time
from commonhelper import *

container_list = [
    '--influx-test-db',
    '--splunk-pump',
    '--elk-pump',
    '--timescale-pump',
    '--victoria-pump --victoria-db',
    '--prometheus-test-db'
]
setup_container_list = ['prometheus-test-db','--influx-test-db']

class TestDockerSetup:
    """
        Checks the status of a Docker container.

        Args:
            container_name (str): The name of the Docker container.

        Returns:
            None

        Raises:
            AssertionError: If the container is not up.
    """
    def check_docker_container_status(self, container_name):
        container_name_short = container_name.split()[0].lstrip('-').split('-')[0]
        print(f"Checking status for: {container_name_short}")
        out = local_exec(f"docker ps -a | grep '{container_name_short}'")
        assert 'Up' in out, f"{container_name} container is not up"
        print(f"{container_name} container is up")

    """
        This function tests the setup of a container by calling a setup command from a compose.sh file.
        The function takes a container name as a parameter, which is a value from the setup_container_list list.
        The function returns None.
    """
    @pytest.mark.parametrize("container", setup_container_list)
    def test_build_setup(self, container):
        print(f"Setting up container for option: {container}")
        cmd1 = "../docker-compose-files/compose.sh setup " + container
        out = local_exec(cmd1)
        time.sleep(10)
        assert ("Influx pump container setup done." in out or
                "Prometheus pump container setup done." in out), f"{container} setup failed: {out}"
        print(f"{container} setup done successfully")

    """
        Tests the start and stop functionality of a container.
        Parameters:
            container (str): The name of the container to be tested.
        Return Type:
            None
    """
    @pytest.mark.parametrize("container", container_list)
    def test_docker_compose_start_stop(self, container):
        print(f"Starting container: {container}")
        out = local_exec("../docker-compose-files/compose.sh start " + container)
        time.sleep(5)
        assert "Created" in out or "Started" in out, f"{container} not started successfully: {out}"
        print(f"{container} started successfully")

        self.check_docker_container_status(container)

        out = local_exec("../docker-compose-files/compose.sh stop " + container)
        assert "Stopped" in out, f"{container} did not stop successfully: {out}"
        print(f"{container} stopped successfully")