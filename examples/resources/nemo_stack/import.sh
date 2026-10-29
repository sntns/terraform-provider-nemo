# A stack nemo deployed, on the environment's Docker (DOCKER_HOST, the current docker context)
terraform import nemo_stack.shop shop

# On another Docker: its address, then the stack's name
terraform import nemo_stack.shop tcp://10.1.0.10:2376/shop
