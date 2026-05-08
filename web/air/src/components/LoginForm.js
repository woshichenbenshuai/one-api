import React, { useContext, useEffect, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { UserContext } from '../context/User';
import { API, getLogo, showError, showInfo, showSuccess } from '../helpers';
import { onGitHubOAuthClicked } from './utils';
import Turnstile from 'react-turnstile';
import { Button, Card, Divider, Form, Icon, Layout, Modal } from '@douyinfe/semi-ui';
import Title from '@douyinfe/semi-ui/lib/es/typography/title';
import Text from '@douyinfe/semi-ui/lib/es/typography/text';
import TelegramLoginButton from 'react-telegram-login';

import { IconGithubLogo } from '@douyinfe/semi-icons';
import WeChatIcon from './WeChatIcon';

const LoginForm = () => {
  const [inputs, setInputs] = useState({
    username: '',
    password: '',
    otp_code: '',
    wechat_verification_code: ''
  });
  const [searchParams] = useSearchParams();
  const { username, password, otp_code } = inputs;
  const [, userDispatch] = useContext(UserContext);
  const [turnstileEnabled, setTurnstileEnabled] = useState(false);
  const [turnstileSiteKey, setTurnstileSiteKey] = useState('');
  const [turnstileToken, setTurnstileToken] = useState('');
  const [status, setStatus] = useState({});
  const [showWeChatLoginModal, setShowWeChatLoginModal] = useState(false);
  const navigate = useNavigate();
  const logo = getLogo();

  useEffect(() => {
    if (searchParams.get('expired')) {
      showError('Login expired, please sign in again.');
    }
    let localStatus = localStorage.getItem('status');
    if (localStatus) {
      localStatus = JSON.parse(localStatus);
      setStatus(localStatus);
      if (localStatus.turnstile_check) {
        setTurnstileEnabled(true);
        setTurnstileSiteKey(localStatus.turnstile_site_key);
      }
    }
  }, [searchParams]);

  const handleChange = (name, value) => {
    setInputs((current) => ({ ...current, [name]: value }));
  };

  const onWeChatLoginClicked = () => {
    setShowWeChatLoginModal(true);
  };

  const onSubmitWeChatVerificationCode = async () => {
    if (turnstileEnabled && turnstileToken === '') {
      showInfo('Turnstile is still verifying the browser, please retry in a moment.');
      return;
    }
    const res = await API.get(`/api/oauth/wechat?code=${inputs.wechat_verification_code}`);
    const { success, message, data } = res.data;
    if (success) {
      userDispatch({ type: 'login', payload: data });
      localStorage.setItem('user', JSON.stringify(data));
      navigate('/');
      showSuccess('Login successful.');
      setShowWeChatLoginModal(false);
    } else {
      showError(message);
    }
  };

  const handleSubmit = async () => {
    if (turnstileEnabled && turnstileToken === '') {
      showInfo('Turnstile is still verifying the browser, please retry in a moment.');
      return;
    }
    if (!username || !password) {
      showError('Please enter your username and password.');
      return;
    }
    const res = await API.post(`/api/user/login?turnstile=${turnstileToken}`, {
      username,
      password,
      otp_code
    });
    const { success, message, data } = res.data;
    if (success) {
      userDispatch({ type: 'login', payload: data });
      localStorage.setItem('user', JSON.stringify(data));
      showSuccess('Login successful.');
      navigate('/token');
    } else {
      showError(message);
    }
  };

  const onTelegramLoginClicked = async (response) => {
    const fields = ['id', 'first_name', 'last_name', 'username', 'photo_url', 'auth_date', 'hash', 'lang'];
    const params = {};
    fields.forEach((field) => {
      if (response[field]) {
        params[field] = response[field];
      }
    });
    const res = await API.get(`/api/oauth/telegram/login`, { params });
    const { success, message, data } = res.data;
    if (success) {
      userDispatch({ type: 'login', payload: data });
      localStorage.setItem('user', JSON.stringify(data));
      showSuccess('Login successful.');
      navigate('/');
    } else {
      showError(message);
    }
  };

  return (
    <div>
      <Layout>
        <Layout.Header />
        <Layout.Content>
          <div style={{ justifyContent: 'center', display: 'flex', marginTop: 120 }}>
            <div style={{ width: 500 }}>
              <Card>
                <div style={{ display: 'flex', justifyContent: 'center', marginBottom: 12 }}>
                  <img src={logo} alt="logo" style={{ maxHeight: 64 }} />
                </div>
                <Title heading={2} style={{ textAlign: 'center' }}>
                  User Login
                </Title>
                <Form>
                  <Form.Input
                    field={'username'}
                    label={'Username / Email'}
                    placeholder="Username / Email"
                    name="username"
                    onChange={(value) => handleChange('username', value)}
                  />
                  <Form.Input
                    field={'password'}
                    label={'Password'}
                    placeholder="Password"
                    name="password"
                    type="password"
                    onChange={(value) => handleChange('password', value)}
                  />
                  <Form.Input
                    field={'otp_code'}
                    label={'Authenticator Code'}
                    placeholder="Authenticator Code"
                    name="otp_code"
                    onChange={(value) => handleChange('otp_code', value)}
                  />

                  <Button theme="solid" style={{ width: '100%' }} type={'primary'} size="large" htmlType={'submit'} onClick={handleSubmit}>
                    Login
                  </Button>
                </Form>
                <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: 20 }}>
                  <Text>
                    No account yet? <Link to="/register">Register</Link>
                  </Text>
                  <Text>
                    Forgot password? <Link to="/reset">Reset</Link>
                  </Text>
                </div>
                {status.github_oauth || status.wechat_login || status.telegram_oauth ? (
                  <>
                    <Divider margin="12px" align="center">
                      Other login methods
                    </Divider>
                    <div style={{ display: 'flex', justifyContent: 'center', marginTop: 20 }}>
                      {status.github_oauth ? (
                        <Button
                          type="primary"
                          icon={<IconGithubLogo />}
                          onClick={() => onGitHubOAuthClicked(status.github_client_id)}
                        />
                      ) : null}
                      {status.wechat_login ? (
                        <Button
                          type="primary"
                          style={{ color: 'rgba(var(--semi-green-5), 1)' }}
                          icon={<Icon svg={<WeChatIcon />} />}
                          onClick={onWeChatLoginClicked}
                        />
                      ) : null}
                      {status.telegram_oauth ? (
                        <TelegramLoginButton dataOnauth={onTelegramLoginClicked} botName={status.telegram_bot_name} />
                      ) : null}
                    </div>
                  </>
                ) : null}
                <Modal
                  title="WeChat Login"
                  visible={showWeChatLoginModal}
                  maskClosable={true}
                  onOk={onSubmitWeChatVerificationCode}
                  onCancel={() => setShowWeChatLoginModal(false)}
                  okText={'Login'}
                  size={'small'}
                  centered={true}
                >
                  <div style={{ display: 'flex', alignItem: 'center', flexDirection: 'column' }}>
                    <img src={status.wechat_qrcode} alt="wechat qr" />
                  </div>
                  <div style={{ textAlign: 'center' }}>
                    <p>Scan the QR code and enter the verification code from WeChat.</p>
                  </div>
                  <Form size="large">
                    <Form.Input
                      field={'wechat_verification_code'}
                      placeholder="Verification Code"
                      label={'Verification Code'}
                      value={inputs.wechat_verification_code}
                      onChange={(value) => handleChange('wechat_verification_code', value)}
                    />
                  </Form>
                </Modal>
              </Card>
              {turnstileEnabled ? (
                <div style={{ display: 'flex', justifyContent: 'center', marginTop: 20 }}>
                  <Turnstile
                    sitekey={turnstileSiteKey}
                    onVerify={(token) => {
                      setTurnstileToken(token);
                    }}
                  />
                </div>
              ) : null}
            </div>
          </div>
        </Layout.Content>
      </Layout>
    </div>
  );
};

export default LoginForm;
