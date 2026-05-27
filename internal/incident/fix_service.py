
import os

file_path = "internal/incident/service.go"
with open(file_path, "r") as f:
    content = f.read()

# Methods to update: ResolveByID, ResolveWithAnalysis, ResolveByIDWithAnalysis
# We look for the pattern: 
# if err := s.store.Save(target); err != nil {
#     return incident, nil, err
# }
# AND
# if err := s.store.Save(*current); err != nil {
#     return incident, warnings, err
# }

replacements = [
    (
        "\tif err := s.store.Save(target); err != nil {\n\t\treturn incident, nil, err\n\t}",
        "\tif err := s.store.Save(target); err != nil {\n\t\treturn incident, nil, err\n\t}\n\n\t// \U0001f680 NEW: Persist full incident lifecycle context\n\ts.persistLifecycle(target, now)"
    ),
    (
        "\tif err := s.store.Save(*current); err != nil {\n\t\treturn incident, warnings, err\n\t}",
        "\tif err := s.store.Save(*current); err != nil {\n\t\treturn incident, warnings, err\n\t}\n\n\t// \U0001f680 NEW: Persist full incident lifecycle context\n\ts.persistLifecycle(*current, now)"
    )
]

new_content = content
for old, new in replacements:
    # We want to replace all occurrences EXCEPT the ones already in methods that have it (if any)
    # Actually Resolve (the main one) already has it for *current.
    # Grep showed 4 matches for target and 3 for *current.
    new_content = new_content.replace(old, new)

# Count how many changes were made
if new_content != content:
    with open(file_path, "w") as f:
        f.write(new_content)
    print("Successfully updated service.go")
else:
    print("No changes made. Pattern not found.")
