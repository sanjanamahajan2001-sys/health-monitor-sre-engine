import sys

with open("test/profile_integration_test.go", "r") as f:
    lines = f.readlines()

new_lines = []
for line in lines:
    new_lines.append(line)
    if "exec.Command" in line and "cmd :=" in line:
        indent = line[:len(line) - len(line.lstrip())]
        new_lines.append(indent + "cmd.Dir = \"..\"\n")

with open("test/profile_integration_test.go", "w") as f:
    f.writelines(new_lines)
