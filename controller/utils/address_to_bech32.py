import json

from eth_utils.hexadecimal import remove_0x_prefix
from platon_keys.utils.address import MIANNETHRP, TESTNETHRP
from platon_keys.utils.bech32 import encode

if __name__ == "__main__":
    genesis = {}
    with open("../tmpl/genesis.json", 'r', encoding="UTF-8") as f:
        genesis = json.load(f)
    alloc = {}
    addresses = []
    with open("../tmpl/all_address.json", "r", encoding="UTF-8") as f:
        addresses = json.load(f)
    new_addresses = []
    for address in addresses:
        addr = encode(TESTNETHRP, list(bytes.fromhex(
            remove_0x_prefix(address["address"]))))
        address["address"] = addr
        new_addresses.append(address)
        alloc[addr] = {
            "balance": "0x200000000000000000000000000000000000000000000000000000000000"}
    with open("all_addr_and_private_keys.json", "w", encoding="UTF-8") as f:
        f.write(json.dumps(new_addresses, indent=2))
    genesis["alloc"] = alloc
    with open("genesis.json", "w", encoding="UTF-8") as f:
        f.write(json.dumps(genesis, indent=2))
