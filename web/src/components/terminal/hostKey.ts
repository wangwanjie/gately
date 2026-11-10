// 主机密钥类型的显示名，写法与 ssh-keygen -l 一致，与后端 hostKeyTypeLabel 保持相同的映射。

const certSuffix = '-cert-v01@openssh.com'

// plainLabel 返回普通主机密钥类型的短名，未知类型原样返回
function plainLabel(typ: string): string {
  if (typ === 'ssh-ed25519') {
    return 'ED25519'
  }
  if (typ === 'ssh-rsa') {
    return 'RSA'
  }
  if (typ.startsWith('ecdsa-sha2-')) {
    return 'ECDSA'
  }
  return typ
}

// hostKeyTypeLabel 返回主机密钥类型的短名；主机证书（如 ssh-ed25519-cert-v01@openssh.com）显示为 ED25519-CERT
export function hostKeyTypeLabel(typ: string): string {
  if (typ.endsWith(certSuffix)) {
    const base = typ.slice(0, -certSuffix.length)
    const label = plainLabel(base)
    return label === base ? typ : `${label}-CERT`
  }
  return plainLabel(typ)
}
