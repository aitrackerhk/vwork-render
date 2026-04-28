import json

with open('zh.json', 'r', encoding='utf-8') as f:
    zh = json.load(f)

with open('en.json', 'r', encoding='utf-8') as f:
    en = json.load(f)

def find_missing_keys(d1, d2, path=''):
    missing = []
    for k in d1:
        p = f'{path}.{k}' if path else k
        if k not in d2:
            missing.append((p, 'KEY_MISSING'))
        elif isinstance(d1[k], dict) and isinstance(d2.get(k), dict):
            missing.extend(find_missing_keys(d1[k], d2[k], p))
        elif isinstance(d1[k], dict) and not isinstance(d2.get(k), dict):
            missing.append((p, 'TYPE_MISMATCH'))
    return missing

def find_empty_values(d1, d2, path=''):
    empty = []
    for k in d1:
        p = f'{path}.{k}' if path else k
        if k in d2:
            if isinstance(d1[k], dict) and isinstance(d2[k], dict):
                empty.extend(find_empty_values(d1[k], d2[k], p))
            elif d2[k] == '' or d2[k] is None:
                empty.append((p, 'EMPTY_VALUE'))
    return empty

missing = find_missing_keys(zh, en)
empty = find_empty_values(zh, en)

print("=== Missing Keys ===")
for m in sorted(set(missing)):
    print(f"{m[0]} - {m[1]}")

print("\n=== Empty Values ===")
for e in sorted(set(empty)):
    print(f"{e[0]} - {e[1]}")
