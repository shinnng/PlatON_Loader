#!/usr/bin/env bash
#

set -e

BATCH_BIN=/usr/local/bin/batch

STAKING=
if [ "${STAKING_FLAG}" = "true" ]; then
	STAKING="-staking=true"
fi

ONLY_CONSENSUS=
if [ "${ONLY_CONSENSUS_FLAG}" = "true" ]; then
	ONLY_CONSENSUS="-only-consensus=true"
fi

DELEGATE=
if [ "${DELEGATE_FLAG}" = "true" ]; then
	DELEGATE="-delegate=true"
fi

R_ACCOUNTS_FLAG=
if [ "${R_ACCOUNTS}" = "true" ]; then
	R_ACCOUNTS_FLAG="--rand_accounts=/data/1m_accounts.json"
fi

${BATCH_BIN} -cmd ${USE_CMD} \
	-url ${URL} \
	-accounts /data/all_addr_and_private_keys.json \
	-interval_ms ${INTERVAL} \
	-idx ${IDX} \
	-count ${COUNT} \
	-nm ${NODENAME} \
	-nodekey ${NODEKEY} \
	-blskey ${BLSKEY} \
	-rand_count ${RAND_COUNT} \
	-chain_id ${CHAINID} \
	-rand_idx ${R_IDX} \
	-private_key ${PRIVATE_KEY} \
	-program_version ${PROGRAM_VERSION} \
    -delegate_nodes /data/delegate_nodes.txt \
    ${STAKING} ${ONLY_CONSENSUS} ${DELEGATE} ${R_ACCOUNTS_FLAG}
