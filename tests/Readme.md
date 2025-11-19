## Introduction
This folder contains a collection of tests for telemetry reference tool
## Installation
To install the required packages, run the following commands:

## Install Pytest
```bash
pip install pytest

pip install pytest-html
```
## Running Tests
To run the tests, use the following command:
``` bash
python3 -m pytest test_docker_container.py --html=report.html --self-contained-html
