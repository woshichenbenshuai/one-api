import React, { useContext, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Divider,
  Form,
  Header,
  Image,
  Message,
  Modal,
} from 'semantic-ui-react';
import { Link, useNavigate } from 'react-router-dom';
import {
  API,
  copy,
  showError,
  showInfo,
  showNotice,
  showSuccess,
} from '../helpers';
import Turnstile from 'react-turnstile';
import { UserContext } from '../context/User';
import { onGitHubOAuthClicked, onLarkOAuthClicked } from './utils';

const PersonalSetting = () => {
  const { t } = useTranslation();
  const [userState, userDispatch] = useContext(UserContext);
  let navigate = useNavigate();

  const [inputs, setInputs] = useState({
    wechat_verification_code: '',
    email_verification_code: '',
    email: '',
    self_account_deletion_confirmation: '',
    authenticator_code: '',
    disable_authenticator_code: '',
  });
  const [status, setStatus] = useState({});
  const [showWeChatBindModal, setShowWeChatBindModal] = useState(false);
  const [showEmailBindModal, setShowEmailBindModal] = useState(false);
  const [showAccountDeleteModal, setShowAccountDeleteModal] = useState(false);
  const [showAuthenticatorSetupModal, setShowAuthenticatorSetupModal] =
    useState(false);
  const [showAuthenticatorDisableModal, setShowAuthenticatorDisableModal] =
    useState(false);
  const [turnstileEnabled, setTurnstileEnabled] = useState(false);
  const [turnstileSiteKey, setTurnstileSiteKey] = useState('');
  const [turnstileToken, setTurnstileToken] = useState('');
  const [loading, setLoading] = useState(false);
  const [disableButton, setDisableButton] = useState(false);
  const [countdown, setCountdown] = useState(30);
  const [affLink, setAffLink] = useState('');
  const [systemToken, setSystemToken] = useState('');
  const [authenticatorStatus, setAuthenticatorStatus] = useState({
    enabled: false,
    secret_configured: false,
  });
  const [authenticatorSecret, setAuthenticatorSecret] = useState('');
  const [authenticatorURI, setAuthenticatorURI] = useState('');

  useEffect(() => {
    let status = localStorage.getItem('status');
    if (status) {
      status = JSON.parse(status);
      setStatus(status);
      if (status.turnstile_check) {
        setTurnstileEnabled(true);
        setTurnstileSiteKey(status.turnstile_site_key);
      }
    }
    loadAuthenticatorStatus().then();
  }, []);

  useEffect(() => {
    let countdownInterval = null;
    if (disableButton && countdown > 0) {
      countdownInterval = setInterval(() => {
        setCountdown(countdown - 1);
      }, 1000);
    } else if (countdown === 0) {
      setDisableButton(false);
      setCountdown(30);
    }
    return () => clearInterval(countdownInterval); // Clean up on unmount
  }, [disableButton, countdown]);

  const handleInputChange = (e, { name, value }) => {
    setInputs((inputs) => ({ ...inputs, [name]: value }));
  };

  const loadAuthenticatorStatus = async () => {
    const res = await API.get('/api/user/self/authenticator');
    const { success, message, data } = res.data;
    if (success) {
      setAuthenticatorStatus(data);
    } else {
      showError(message);
    }
  };

  const generateAccessToken = async () => {
    const res = await API.get('/api/user/token');
    const { success, message, data } = res.data;
    if (success) {
      setSystemToken(data);
      setAffLink('');
      await copy(data);
      showSuccess(`令牌已重置并已复制到剪贴板`);
    } else {
      showError(message);
    }
  };

  const getAffLink = async () => {
    const res = await API.get('/api/user/aff');
    const { success, message, data } = res.data;
    if (success) {
      let link = `${window.location.origin}/register?aff=${data}`;
      setAffLink(link);
      setSystemToken('');
      await copy(link);
      showSuccess(`邀请链接已复制到剪切板`);
    } else {
      showError(message);
    }
  };

  const handleAffLinkClick = async (e) => {
    e.target.select();
    await copy(e.target.value);
    showSuccess(`邀请链接已复制到剪切板`);
  };

  const handleSystemTokenClick = async (e) => {
    e.target.select();
    await copy(e.target.value);
    showSuccess(`系统令牌已复制到剪切板`);
  };

  const deleteAccount = async () => {
    if (inputs.self_account_deletion_confirmation !== userState.user.username) {
      showError('请输入你的账户名以确认删除！');
      return;
    }

    const res = await API.delete('/api/user/self');
    const { success, message } = res.data;

    if (success) {
      showSuccess('账户已删除！');
      await API.get('/api/user/logout');
      userDispatch({ type: 'logout' });
      localStorage.removeItem('user');
      navigate('/login');
    } else {
      showError(message);
    }
  };

  const setupAuthenticator = async () => {
    const res = await API.post('/api/user/self/authenticator/setup');
    const { success, message, data } = res.data;
    if (success) {
      setAuthenticatorSecret(data.secret);
      setAuthenticatorURI(data.otpauth_uri);
      setInputs((current) => ({ ...current, authenticator_code: '' }));
      setShowAuthenticatorSetupModal(true);
      setAuthenticatorStatus({
        enabled: false,
        secret_configured: true,
      });
    } else {
      showError(message);
    }
  };

  const enableAuthenticator = async () => {
    if (!inputs.authenticator_code) {
      showError('Please enter the authenticator code.');
      return;
    }
    const res = await API.post('/api/user/self/authenticator/enable', {
      otp_code: inputs.authenticator_code,
    });
    const { success, message } = res.data;
    if (success) {
      showSuccess('Authenticator enabled.');
      setShowAuthenticatorSetupModal(false);
      setAuthenticatorSecret('');
      setAuthenticatorURI('');
      setInputs((current) => ({ ...current, authenticator_code: '' }));
      await loadAuthenticatorStatus();
    } else {
      showError(message);
    }
  };

  const disableAuthenticator = async () => {
    if (!inputs.disable_authenticator_code) {
      showError('Please enter the authenticator code.');
      return;
    }
    const res = await API.post('/api/user/self/authenticator/disable', {
      otp_code: inputs.disable_authenticator_code,
    });
    const { success, message } = res.data;
    if (success) {
      showSuccess('Authenticator disabled.');
      setShowAuthenticatorDisableModal(false);
      setInputs((current) => ({ ...current, disable_authenticator_code: '' }));
      await loadAuthenticatorStatus();
    } else {
      showError(message);
    }
  };

  const bindWeChat = async () => {
    if (inputs.wechat_verification_code === '') return;
    const res = await API.get(
      `/api/oauth/wechat/bind?code=${inputs.wechat_verification_code}`
    );
    const { success, message } = res.data;
    if (success) {
      showSuccess('微信账户绑定成功！');
      setShowWeChatBindModal(false);
    } else {
      showError(message);
    }
  };

  const sendVerificationCode = async () => {
    setDisableButton(true);
    if (inputs.email === '') return;
    if (turnstileEnabled && turnstileToken === '') {
      showInfo('请稍后几秒重试，Turnstile 正在检查用户环境！');
      return;
    }
    setLoading(true);
    const res = await API.get(
      `/api/verification?email=${inputs.email}&turnstile=${turnstileToken}`
    );
    const { success, message } = res.data;
    if (success) {
      showSuccess('验证码发送成功，请检查邮箱！');
    } else {
      showError(message);
    }
    setLoading(false);
  };

  const bindEmail = async () => {
    if (inputs.email_verification_code === '') return;
    setLoading(true);
    const res = await API.get(
      `/api/oauth/email/bind?email=${inputs.email}&code=${inputs.email_verification_code}`
    );
    const { success, message } = res.data;
    if (success) {
      showSuccess('邮箱账户绑定成功！');
      setShowEmailBindModal(false);
    } else {
      showError(message);
    }
    setLoading(false);
  };

  return (
    <div style={{ lineHeight: '40px' }}>
      <Header as='h3'>{t('setting.personal.general.title')}</Header>
      <Message>{t('setting.personal.general.system_token_notice')}</Message>
      <Button as={Link} to={`/user/edit/`}>
        {t('setting.personal.general.buttons.update_profile')}
      </Button>
      <Button onClick={generateAccessToken}>
        {t('setting.personal.general.buttons.generate_token')}
      </Button>
      <Button onClick={getAffLink}>
        {t('setting.personal.general.buttons.copy_invite')}
      </Button>
      <Button
        onClick={() => {
          setShowAccountDeleteModal(true);
        }}
      >
        {t('setting.personal.general.buttons.delete_account')}
      </Button>

      {systemToken && (
        <Form.Input
          fluid
          readOnly
          value={systemToken}
          onClick={handleSystemTokenClick}
          style={{ marginTop: '10px' }}
        />
      )}
      {affLink && (
        <Form.Input
          fluid
          readOnly
          value={affLink}
          onClick={handleAffLinkClick}
          style={{ marginTop: '10px' }}
        />
      )}
      <Divider />
      <Header as='h3'>Authenticator</Header>
      <Message>
        {authenticatorStatus.enabled
          ? 'Authenticator login is enabled for this account.'
          : 'Configure TOTP-based authenticator login here.'}
      </Message>
      <Button onClick={setupAuthenticator}>Configure Authenticator</Button>
      {authenticatorStatus.enabled && (
        <Button onClick={() => setShowAuthenticatorDisableModal(true)}>
          Disable Authenticator
        </Button>
      )}
      <Modal
        onClose={() => setShowAuthenticatorSetupModal(false)}
        onOpen={() => setShowAuthenticatorSetupModal(true)}
        open={showAuthenticatorSetupModal}
        size={'small'}
      >
        <Modal.Header>Configure Authenticator</Modal.Header>
        <Modal.Content>
          <Modal.Description>
            <Message>
              Import the secret below into your authenticator app, then enter
              the current 6-digit code to enable it.
            </Message>
            <Form size='large'>
              <Form.Input
                fluid
                readOnly
                label='Secret'
                value={authenticatorSecret}
                onClick={async () => {
                  if (authenticatorSecret) {
                    await copy(authenticatorSecret);
                    showSuccess('Secret copied to clipboard.');
                  }
                }}
              />
              <Form.TextArea
                readOnly
                label='OTPAuth URI'
                value={authenticatorURI}
                style={{ minHeight: '100px' }}
              />
              <Form.Input
                fluid
                label='Code'
                placeholder='Enter the current authenticator code'
                name='authenticator_code'
                value={inputs.authenticator_code || ''}
                onChange={handleInputChange}
              />
              <Button color='' fluid size='large' onClick={enableAuthenticator}>
                Enable Authenticator
              </Button>
            </Form>
          </Modal.Description>
        </Modal.Content>
      </Modal>
      <Modal
        onClose={() => setShowAuthenticatorDisableModal(false)}
        onOpen={() => setShowAuthenticatorDisableModal(true)}
        open={showAuthenticatorDisableModal}
        size={'tiny'}
      >
        <Modal.Header>Disable Authenticator</Modal.Header>
        <Modal.Content>
          <Modal.Description>
            <Form size='large'>
              <Form.Input
                fluid
                label='Code'
                placeholder='Enter the current authenticator code'
                name='disable_authenticator_code'
                value={inputs.disable_authenticator_code || ''}
                onChange={handleInputChange}
              />
              <Button color='red' fluid size='large' onClick={disableAuthenticator}>
                Disable Authenticator
              </Button>
            </Form>
          </Modal.Description>
        </Modal.Content>
      </Modal>
      <Divider />
      <Header as='h3'>{t('setting.personal.binding.title')}</Header>
      {status.wechat_login && (
        <Button onClick={() => setShowWeChatBindModal(true)}>
          {t('setting.personal.binding.buttons.bind_wechat')}
        </Button>
      )}
      <Modal
        onClose={() => setShowWeChatBindModal(false)}
        onOpen={() => setShowWeChatBindModal(true)}
        open={showWeChatBindModal}
        size={'mini'}
      >
        <Modal.Content>
          <Modal.Description>
            <Image src={status.wechat_qrcode} fluid />
            <div style={{ textAlign: 'center' }}>
              <p>{t('setting.personal.binding.wechat.description')}</p>
            </div>
            <Form size='large'>
              <Form.Input
                fluid
                placeholder={t(
                  'setting.personal.binding.wechat.verification_code'
                )}
                name='wechat_verification_code'
                value={inputs.wechat_verification_code}
                onChange={handleInputChange}
              />
              <Button color='' fluid size='large' onClick={bindWeChat}>
                {t('setting.personal.binding.wechat.bind')}
              </Button>
            </Form>
          </Modal.Description>
        </Modal.Content>
      </Modal>
      {status.github_oauth && (
        <Button onClick={() => onGitHubOAuthClicked(status.github_client_id)}>
          {t('setting.personal.binding.buttons.bind_github')}
        </Button>
      )}
      {status.lark_client_id && (
        <Button onClick={() => onLarkOAuthClicked(status.lark_client_id)}>
          {t('setting.personal.binding.buttons.bind_lark')}
        </Button>
      )}
      <Button onClick={() => setShowEmailBindModal(true)}>
        {t('setting.personal.binding.buttons.bind_email')}
      </Button>
      <Modal
        onClose={() => setShowEmailBindModal(false)}
        onOpen={() => setShowEmailBindModal(true)}
        open={showEmailBindModal}
        size={'tiny'}
        style={{ maxWidth: '450px' }}
      >
        <Modal.Header>{t('setting.personal.binding.email.title')}</Modal.Header>
        <Modal.Content>
          <Modal.Description>
            <Form size='large'>
              <Form.Input
                fluid
                placeholder={t(
                  'setting.personal.binding.email.email_placeholder'
                )}
                onChange={handleInputChange}
                name='email'
                type='email'
                action={
                  <Button
                    onClick={sendVerificationCode}
                    disabled={disableButton || loading}
                  >
                    {disableButton
                      ? t('setting.personal.binding.email.get_code_retry', {
                          countdown,
                        })
                      : t('setting.personal.binding.email.get_code')}
                  </Button>
                }
              />
              <Form.Input
                fluid
                placeholder={t(
                  'setting.personal.binding.email.code_placeholder'
                )}
                name='email_verification_code'
                value={inputs.email_verification_code}
                onChange={handleInputChange}
              />
              {turnstileEnabled && (
                <Turnstile
                  sitekey={turnstileSiteKey}
                  onVerify={(token) => {
                    setTurnstileToken(token);
                  }}
                />
              )}
              <div
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  marginTop: '1rem',
                }}
              >
                <Button
                  color=''
                  fluid
                  size='large'
                  onClick={bindEmail}
                  loading={loading}
                >
                  {t('setting.personal.binding.email.bind')}
                </Button>
                <div style={{ width: '1rem' }}></div>
                <Button
                  fluid
                  size='large'
                  onClick={() => setShowEmailBindModal(false)}
                >
                  {t('setting.personal.binding.email.cancel')}
                </Button>
              </div>
            </Form>
          </Modal.Description>
        </Modal.Content>
      </Modal>
      <Modal
        onClose={() => setShowAccountDeleteModal(false)}
        onOpen={() => setShowAccountDeleteModal(true)}
        open={showAccountDeleteModal}
        size={'tiny'}
        style={{ maxWidth: '450px' }}
      >
        <Modal.Header>
          {t('setting.personal.delete_account.title')}
        </Modal.Header>
        <Modal.Content>
          <Message>{t('setting.personal.delete_account.warning')}</Message>
          <Modal.Description>
            <Form size='large'>
              <Form.Input
                fluid
                placeholder={t(
                  'setting.personal.delete_account.confirm_placeholder',
                  {
                    username: userState?.user?.username,
                  }
                )}
                name='self_account_deletion_confirmation'
                value={inputs.self_account_deletion_confirmation}
                onChange={handleInputChange}
              />
              {turnstileEnabled && (
                <Turnstile
                  sitekey={turnstileSiteKey}
                  onVerify={(token) => {
                    setTurnstileToken(token);
                  }}
                />
              )}
              <div
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  marginTop: '1rem',
                }}
              >
                <Button
                  color='red'
                  fluid
                  size='large'
                  onClick={deleteAccount}
                  loading={loading}
                >
                  {t('setting.personal.delete_account.buttons.confirm')}
                </Button>
                <div style={{ width: '1rem' }}></div>
                <Button
                  fluid
                  size='large'
                  onClick={() => setShowAccountDeleteModal(false)}
                >
                  {t('setting.personal.delete_account.buttons.cancel')}
                </Button>
              </div>
            </Form>
          </Modal.Description>
        </Modal.Content>
      </Modal>
    </div>
  );
};

export default PersonalSetting;
