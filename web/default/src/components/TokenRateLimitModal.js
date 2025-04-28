import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Modal,
  Form,
  Button,
  Message
} from 'semantic-ui-react';
import { API, showError, showSuccess } from '../helpers';

const TokenRateLimitModal = ({ open, tokenId, tokenName, onClose, onSuccess }) => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  
  // 默认初始值
  const defaultInputs = {
    token_id: tokenId,
    max_qps: 0,
    daily_quota: 0,
    enabled: false
  };
  
  const [inputs, setInputs] = useState(defaultInputs);

  const handleInputChange = (e, { name, value }) => {
    const numericFields = ['max_qps', 'daily_quota'];
    if (numericFields.includes(name)) {
      const numericValue = value === '' ? 0 : Number(value);
      setInputs((inputs) => ({ ...inputs, [name]: numericValue }));
    } else {
      setInputs((inputs) => ({ ...inputs, [name]: value }));
    }
  };

  const fetchTokenRateLimit = async () => {
    if (!tokenId) return;
    
    // 重置为默认值，确保切换Token时不保留之前的数据
    setInputs({
      ...defaultInputs,
      token_id: tokenId
    });
    
    setLoading(true);
    try {
      const res = await API.get(`/api/rate-limit/token/${tokenId}`);
      const { success, message, data } = res.data;
      if (success && data) {
        const formattedData = { ...data.rate_limit };
        
        formattedData.max_qps = Number(formattedData.max_qps || 0);
        formattedData.daily_quota = Number(formattedData.daily_quota || 0);
        
        setInputs(formattedData);
      } else {
        console.log("Failed to fetch rate limit: " + message);
        // 保留重置后的默认值，不需要额外操作
      }
    } catch (error) {
      console.log("Error fetching rate limit: " + error.message);
      // 保留重置后的默认值，不需要额外操作
    } finally {
      setLoading(false);
    }
  };

  const handleSubmit = async () => {
    setSaving(true);
    try {
      const submitData = { 
        token_id: inputs.token_id || tokenId,
        enabled: inputs.enabled,
        max_qps: Number(inputs.max_qps || 0),
        daily_quota: Number(inputs.daily_quota || 0)
      };
      
      const res = await API.put(`/api/rate-limit/token/${tokenId}`, submitData);
      const { success, message } = res.data;
      if (success) {
        showSuccess(t('setting.rate_limit.update_success'));
        if (onSuccess) onSuccess();
        onClose();
      } else {
        showError(message || t('setting.rate_limit.update_failed'));
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setSaving(false);
    }
  };

  // 每次打开对话框或者tokenId变化时重新获取数据
  useEffect(() => {
    if (open && tokenId) {
      fetchTokenRateLimit();
    }
  }, [open, tokenId]);
  
  // 当对话框关闭时重置状态
  useEffect(() => {
    if (!open) {
      setInputs(defaultInputs);
    }
  }, [open]);

  return (
    <Modal open={open} onClose={onClose} size='small'>
      <Modal.Header>
        {t('token.rate_limit.title')} - {tokenName}
      </Modal.Header>
      <Modal.Content>
        <Form loading={loading}>
          <Form.Field>
            <label>{t('token.rate_limit.enabled')}</label>
            <Form.Checkbox
              toggle
              name='enabled'
              checked={inputs.enabled}
              onChange={(e, { name, checked }) => handleInputChange(e, { name, value: checked })}
            />
            <small>{t('token.rate_limit.enabled_hint')}</small>
          </Form.Field>
          <Form.Field>
            <label>{t('token.rate_limit.max_qps')}</label>
            <Form.Input
              name='max_qps'
              type='number'
              min={0}
              placeholder={t('token.rate_limit.max_qps_placeholder')}
              onChange={handleInputChange}
              value={inputs.max_qps}
            />
            <small>{t('token.rate_limit.max_qps_hint')}</small>
          </Form.Field>
          <Form.Field>
            <label>{t('token.rate_limit.daily_quota')}</label>
            <Form.Input
              name='daily_quota'
              type='number'
              min={0}
              placeholder={t('token.rate_limit.daily_quota_placeholder')}
              onChange={handleInputChange}
              value={inputs.daily_quota}
            />
            <small>{t('token.rate_limit.daily_quota_hint')}</small>
          </Form.Field>
        </Form>
      </Modal.Content>
      <Modal.Actions>
        <Button onClick={onClose}>
          {t('setting.rate_limit.cancel')}
        </Button>
        <Button primary loading={saving} onClick={handleSubmit}>
          {t('setting.rate_limit.save')}
        </Button>
      </Modal.Actions>
    </Modal>
  );
};

export default TokenRateLimitModal;
