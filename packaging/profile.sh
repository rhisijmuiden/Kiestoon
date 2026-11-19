# Shows how to reach the Kiestoon portal when root logs in on the console.
if [ "$(id -u)" = "0" ] && [ -t 1 ] && command -v kiestoon >/dev/null 2>&1; then
  _kt_ip=$(hostname -I 2>/dev/null | awk '{print $1}')
  printf '\n  \033[1;36mKiestoon\033[0m %s\n' "$(kiestoon version 2>/dev/null | awk '{print $2}')"
  printf '  Portal: http://%s:8080\n' "${_kt_ip:-<this-machine>}"
  if [ -s /var/lib/kiestoon/setup-token ]; then
    printf '  Setup token: \033[1m%s\033[0m\n' "$(cat /var/lib/kiestoon/setup-token)"
  fi
  printf '\n'
  unset _kt_ip
fi
