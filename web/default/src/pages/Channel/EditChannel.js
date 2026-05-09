import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button, Card, Form } from 'semantic-ui-react';
import { useNavigate, useParams } from 'react-router-dom';
import { API, showError, showInfo, showSuccess, verifyJSON } from '../../helpers';

const MODEL_MAPPING_EXAMPLE = {
  'gpt-3.5-turbo-0301': 'gpt-3.5-turbo',
  'gpt-4-0314': 'gpt-4',
  'gpt-4-32k-0314': 'gpt-4-32k',
};

const CUSTOM_CHANNEL_TYPE = 8;
const CUSTOM_CHANNEL_OPTIONS = [
  {
    key: CUSTOM_CHANNEL_TYPE,
    text: 'Custom Channel',
    value: CUSTOM_CHANNEL_TYPE,
  },
];

const originInputs = {
  name: '',
  type: CUSTOM_CHANNEL_TYPE,
  key: '',
  base_url: '',
  model_mapping: '',
  system_prompt: '',
  models: [],
  groups: ['default'],
};

const originConfig = {
  responses_compat: false,
};

const EditChannel = () => {
  const { t } = useTranslation();
  const params = useParams();
  const navigate = useNavigate();
  const channelId = params.id;
  const isEdit = channelId !== undefined;
  const [loading, setLoading] = useState(isEdit);
  const [batch, setBatch] = useState(false);
  const [inputs, setInputs] = useState(originInputs);
  const [config, setConfig] = useState(originConfig);
  const [groupOptions, setGroupOptions] = useState([]);

  const modelOptions = inputs.models.map((model) => ({
    key: model,
    text: model,
    value: model,
  }));

  const handleCancel = () => {
    navigate('/channel');
  };

  const handleInputChange = (e, { name, value }) => {
    setInputs((prev) => ({ ...prev, [name]: value }));
  };

  const handleConfigChange = (e, { name, value }) => {
    setConfig((prev) => ({ ...prev, [name]: value }));
  };

  const loadChannel = async () => {
    const res = await API.get(`/api/channel/${channelId}`);
    const { success, message, data } = res.data;
    if (!success) {
      showError(message);
      setLoading(false);
      return;
    }
    setInputs({
      ...originInputs,
      ...data,
      type: CUSTOM_CHANNEL_TYPE,
      models: data.models ? data.models.split(',') : [],
      groups: data.group ? data.group.split(',') : [],
      model_mapping: data.model_mapping
        ? JSON.stringify(JSON.parse(data.model_mapping), null, 2)
        : '',
    });
    setConfig(data.config ? { ...originConfig, ...JSON.parse(data.config) } : originConfig);
    setLoading(false);
  };

  const fetchGroups = async () => {
    try {
      const res = await API.get('/api/group/');
      setGroupOptions(
        res.data.data.map((group) => ({
          key: group,
          text: group,
          value: group,
        }))
      );
    } catch (error) {
      showError(error.message);
    }
  };

  useEffect(() => {
    if (isEdit) {
      loadChannel().then();
    }
    fetchGroups().then();
  }, []);

  const submit = async () => {
    if (!isEdit && (inputs.name === '' || inputs.key === '')) {
      showInfo(t('channel.edit.messages.name_required'));
      return;
    }
    if (inputs.models.length === 0) {
      showInfo(t('channel.edit.messages.models_required'));
      return;
    }
    if (inputs.model_mapping !== '' && !verifyJSON(inputs.model_mapping)) {
      showInfo(t('channel.edit.messages.model_mapping_invalid'));
      return;
    }
    const localInputs = {
      ...inputs,
      type: CUSTOM_CHANNEL_TYPE,
      models: inputs.models.join(','),
      group: inputs.groups.join(','),
      config: JSON.stringify(config),
    };
    if (localInputs.base_url && localInputs.base_url.endsWith('/')) {
      localInputs.base_url = localInputs.base_url.slice(0, localInputs.base_url.length - 1);
    }
    let res;
    if (isEdit) {
      res = await API.put('/api/channel/', {
        ...localInputs,
        id: parseInt(channelId),
      });
    } else {
      res = await API.post('/api/channel/', localInputs);
    }
    const { success, message } = res.data;
    if (!success) {
      showError(message);
      return;
    }
    if (isEdit) {
      showSuccess(t('channel.edit.messages.update_success'));
      return;
    }
    showSuccess(t('channel.edit.messages.create_success'));
    setInputs(originInputs);
    setConfig(originConfig);
    setBatch(false);
  };

  return (
    <div className='dashboard-container'>
      <Card fluid className='chart-card'>
        <Card.Content>
          <Card.Header className='header'>
            {isEdit ? t('channel.edit.title_edit') : t('channel.edit.title_create')}
          </Card.Header>
          <Form loading={loading} autoComplete='new-password'>
            <Form.Field>
              <Form.Select
                label={t('channel.edit.type')}
                name='type'
                required
                options={CUSTOM_CHANNEL_OPTIONS}
                value={inputs.type}
                onChange={handleInputChange}
              />
            </Form.Field>
            <Form.Field>
              <Form.Input
                label={t('channel.edit.name')}
                name='name'
                placeholder={t('channel.edit.name_placeholder')}
                onChange={handleInputChange}
                value={inputs.name}
                required
              />
            </Form.Field>
            <Form.Field>
              <Form.Dropdown
                label={t('channel.edit.group')}
                placeholder={t('channel.edit.group_placeholder')}
                name='groups'
                required
                fluid
                multiple
                selection
                allowAdditions
                additionLabel={t('channel.edit.group_addition')}
                onChange={handleInputChange}
                value={inputs.groups}
                autoComplete='new-password'
                options={groupOptions}
              />
            </Form.Field>
            <Form.Field>
              <Form.Input
                required
                label={t('channel.edit.proxy_url')}
                name='base_url'
                placeholder={t('channel.edit.proxy_url_placeholder')}
                onChange={handleInputChange}
                value={inputs.base_url}
                autoComplete='new-password'
              />
            </Form.Field>
            <Form.Field>
              <Form.Dropdown
                label={t('channel.edit.models')}
                placeholder={t('channel.edit.models_placeholder')}
                name='models'
                required
                fluid
                multiple
                search
                selection
                allowAdditions
                additionLabel='Add model: '
                onAddItem={(e, { value }) => {
                  if (!inputs.models.includes(value)) {
                    handleInputChange(null, {
                      name: 'models',
                      value: [...inputs.models, value],
                    });
                  }
                }}
                onChange={handleInputChange}
                value={inputs.models}
                autoComplete='new-password'
                options={modelOptions}
              />
            </Form.Field>
            <Form.Field>
              <Form.TextArea
                label={t('channel.edit.model_mapping')}
                placeholder={`${t('channel.edit.model_mapping_placeholder')}\n${JSON.stringify(
                  MODEL_MAPPING_EXAMPLE,
                  null,
                  2
                )}`}
                name='model_mapping'
                onChange={handleInputChange}
                value={inputs.model_mapping}
                style={{
                  minHeight: 150,
                  fontFamily: 'JetBrains Mono, Consolas',
                }}
                autoComplete='new-password'
              />
            </Form.Field>
            <Form.Field>
              <Form.TextArea
                label={t('channel.edit.system_prompt')}
                placeholder={t('channel.edit.system_prompt_placeholder')}
                name='system_prompt'
                onChange={handleInputChange}
                value={inputs.system_prompt}
                style={{
                  minHeight: 150,
                  fontFamily: 'JetBrains Mono, Consolas',
                }}
                autoComplete='new-password'
              />
            </Form.Field>
            {batch ? (
              <Form.Field>
                <Form.TextArea
                  label={t('channel.edit.key')}
                  name='key'
                  required
                  placeholder={t('channel.edit.batch_placeholder')}
                  onChange={handleInputChange}
                  value={inputs.key}
                  style={{
                    minHeight: 150,
                    fontFamily: 'JetBrains Mono, Consolas',
                  }}
                  autoComplete='new-password'
                />
              </Form.Field>
            ) : (
              <Form.Field>
                <Form.Input
                  label={t('channel.edit.key')}
                  name='key'
                  required
                  placeholder={t('channel.edit.key_prompts.default')}
                  onChange={handleInputChange}
                  value={inputs.key}
                  autoComplete='new-password'
                />
              </Form.Field>
            )}
            <Form.Field>
              <Form.Checkbox
                checked={config.responses_compat}
                label='Enable Responses compatibility for this channel'
                name='responses_compat'
                onChange={(e, { checked }) =>
                  handleConfigChange(e, {
                    name: 'responses_compat',
                    value: checked,
                  })
                }
              />
            </Form.Field>
            {!isEdit && (
              <Form.Checkbox
                checked={batch}
                label={t('channel.edit.batch')}
                name='batch'
                onChange={() => setBatch(!batch)}
              />
            )}
            <Button onClick={handleCancel}>{t('channel.edit.buttons.cancel')}</Button>
            <Button type={isEdit ? 'button' : 'submit'} positive onClick={submit}>
              {t('channel.edit.buttons.submit')}
            </Button>
          </Form>
        </Card.Content>
      </Card>
    </div>
  );
};

export default EditChannel;
