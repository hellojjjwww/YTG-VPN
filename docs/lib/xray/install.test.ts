import { describe, it, expect } from 'vitest';
import {
  buildScriptCommand,
  buildDockerRun,
  buildDockerCompose,
  type InstallOptions,
} from './install';

const base: InstallOptions = {
  method: 'script',
  version: '',
  enableFail2ban: true,
  panelPort: '',
  webBasePath: '',
};

describe('buildScriptCommand', () => {
  it('uses the YTG-VPN installer for the latest version', () => {
    expect(buildScriptCommand(base)).toBe(
      'curl -fsSL https://raw.githubusercontent.com/hellojjjwww/YTG-VPN/main/install.sh | sudo bash',
    );
  });

  it('pins a specific version by passing the tag to the installer', () => {
    const cmd = buildScriptCommand({ ...base, version: 'v3.4.1' });
    expect(cmd).toBe(
      'curl -fsSL https://raw.githubusercontent.com/hellojjjwww/YTG-VPN/main/install.sh | sudo bash -s -- v3.4.1',
    );
  });

  it('supports the rolling dev-latest build', () => {
    const cmd = buildScriptCommand({ ...base, version: 'dev-latest' });
    expect(cmd).toBe(
      'curl -fsSL https://raw.githubusercontent.com/hellojjjwww/YTG-VPN/main/install.sh | sudo bash -s -- dev-latest',
    );
  });
});

describe('buildDockerRun', () => {
  it('reflects fail2ban, port, and base path options', () => {
    const cmd = buildDockerRun({
      ...base,
      enableFail2ban: false,
      panelPort: '8443',
      webBasePath: '/panel',
    });
    expect(cmd).toContain('XUI_ENABLE_FAIL2BAN=false');
    expect(cmd).toContain('XUI_PORT=8443');
    expect(cmd).toContain('XUI_INIT_WEB_BASE_PATH=/panel');
    expect(cmd).toContain('ghcr.io/mhsanaei/3x-ui:latest');
    expect(cmd).toContain('-v $PWD/db/:/etc/x-ui/');
  });

  it('omits unset port and path', () => {
    const cmd = buildDockerRun(base);
    expect(cmd).not.toContain('XUI_PORT');
    expect(cmd).not.toContain('XUI_INIT_WEB_BASE_PATH');
  });
});

describe('buildDockerCompose', () => {
  it('produces valid-looking compose with the image and volumes', () => {
    const yaml = buildDockerCompose({ ...base, panelPort: '2096' });
    expect(yaml).toContain('image: ghcr.io/mhsanaei/3x-ui:latest');
    expect(yaml).toContain('network_mode: host');
    expect(yaml).toContain("XUI_PORT: '2096'");
    expect(yaml).toContain('- ./db/:/etc/x-ui/');
  });
});
