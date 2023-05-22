import { ethers } from 'ethers'

import { Time } from '../src'

const config = {
  options: {
    projectName: 'op-goerli-periphery-1',
    organizationID:
      '0x610ec20908f3299004e612b0420c2ae717fde28e13eb4d1057bca1255e515136',
  },
  contracts: {
    Faucet: {
      contract: 'Faucet',
      constructorArgs: {
        _admin: '0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266',
      },
      variables: {
        modules: {
          '{{ GitHubFAM }}': {
            ttl: Time.DAY,
            amount: ethers.utils.parseEther('0.05'),
            name: 'GITHUB_ADMIN_FAM',
            enabled: true,
          },
          '{{ OptimistFAM }}': {
            ttl: Time.DAY,
            amount: ethers.utils.parseEther('1.00'),
            name: 'OPTIMIST_ADMIN_FAM',
            enabled: true,
          },
        },
        timeouts: {},
        nonces: {},
      },
    },
    GitHubFAM: {
      contract: 'AdminFaucetAuthModule',
      kind: 'no-proxy',
      unsafeAllowFlexibleConstructor: true,
      constructorArgs: {
        _admin: '0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266',
        _name: 'GithubFAM',
        _version: '1',
      },
    },
    OptimistFAM: {
      contract: 'AdminFaucetAuthModule',
      kind: 'no-proxy',
      unsafeAllowFlexibleConstructor: true,
      constructorArgs: {
        _admin: '0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266',
        _name: 'OptimistFAM',
        _version: '1',
      },
    },
  },
}

export default config
