import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card, Grid, Form, Button, Dropdown, Search } from 'semantic-ui-react';
import { API } from '../../helpers';
import {
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import axios from 'axios';
import './Dashboard.css';

// 在 Dashboard 组件内添加自定义配置
const chartConfig = {
  lineChart: {
    style: {
      background: '#fff',
      borderRadius: '8px',
    },
    line: {
      strokeWidth: 2,
      dot: false,
      activeDot: { r: 4 },
    },
    grid: {
      vertical: false,
      horizontal: true,
      opacity: 0.1,
    },
  },
  colors: {
    requests: '#4318FF',
    quota: '#00B5D8',
    tokens: '#6C63FF',
  },
  barColors: [
    '#4318FF', // 深紫色
    '#00B5D8', // 青色
    '#6C63FF', // 紫色
    '#05CD99', // 绿色
    '#FFB547', // 橙色
    '#FF5E7D', // 粉色
    '#41B883', // 翠绿
    '#7983FF', // 淡紫
    '#FF8F6B', // 珊瑚色
    '#49BEFF', // 天蓝
  ],
};

// 通用样式
const styles = {
  noDataContainer: {
    height: '200px',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: '#999',
    fontSize: '14px',
    border: '1px dashed #ddd',
    borderRadius: '4px',
    margin: '20px 0',
    backgroundColor: '#fafafa'
  },
  tableHeader: { 
    fontSize: '14px', 
    textAlign: 'center' 
  },
  tooltipContent: {
    background: '#fff',
    border: 'none',
    borderRadius: '8px',
    boxShadow: '0 4px 12px rgba(0,0,0,0.15)',
    padding: '10px',
    fontSize: '14px'
  },
  loadingContainer: { 
    textAlign: 'center', 
    padding: '40px 0' 
  }
};

// 工具函数
const utils = {
  // 时区转换辅助函数
  convertToBeiJingTime: (dateStr) => {
    const normalizedDateStr = dateStr.replace(' ', 'T');
    const date = new Date(normalizedDateStr);
    return new Date(date.getTime() + 8 * 60 * 60 * 1000);
  },
  
  // 格式化日期
  formatDate: (dateStr) => {
    const date = new Date(dateStr);
    return date.toLocaleDateString('zh-CN', {
      month: 'numeric',
      day: 'numeric',
    });
  },
  
  // 生成随机颜色
  getRandomColor: (index) => {
    return chartConfig.barColors[index % chartConfig.barColors.length];
  },
  
  // 构建带过滤条件的API URL
  buildApiUrl: (baseUrl, params = {}, excludeModels = []) => {
    let url = baseUrl;
    const queryParams = [];
    
    // 添加其他参数
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined && value !== '') {
        queryParams.push(`${key}=${encodeURIComponent(value)}`);
      }
    });
    
    // 添加模型排除
    if (excludeModels && excludeModels.length > 0) {
      queryParams.push(`excludeModels=${excludeModels.join(',')}`);
    }
    
    // 组合URL
    if (queryParams.length > 0) {
      url += (url.includes('?') ? '&' : '?') + queryParams.join('&');
    }
    
    return url;
  },
  
  // 通用API请求函数
  fetchApi: async (url, onSuccess, onError = () => {}) => {
    try {
      const response = await API.get(url);
      if (response.data.success) {
        onSuccess(response.data.data || []);
      } else {
        console.error('API request failed:', response.data.message);
        onError(response.data.message);
      }
    } catch (error) {
      console.error('API request error:', error);
      onError(error);
    }
  },
  
  // 计算总计
  calculateTotal: (data, field) => {
    return data.reduce((sum, item) => sum + (item[field] || 0), 0);
  }
};

// 图表配置
const chartConfigs = {
  // X轴配置
  xAxisConfig: {
    dataKey: 'date',
    axisLine: false,
    tickLine: false,
    tick: {
      fontSize: 12,
      fill: '#A3AED0',
      textAnchor: 'middle',
    },
    tickFormatter: utils.formatDate,
    interval: 0,
    minTickGap: 5,
    padding: { left: 30, right: 30 },
  },
  
  // 时间间隔X轴配置
  xAxisIntervalConfig: (timeInterval) => ({
    dataKey: 'interval',
    axisLine: false,
    tickLine: false,
    tick: {
      fontSize: 12,
      fill: '#A3AED0',
      textAnchor: 'middle',
      angle: 20,
      dy: 20
    },
    height: 60,
    tickFormatter: (interval) => {
      try {
        const hourInterval = interval + ":00:00";
        const date = new Date(hourInterval);
        const hours = date.getHours();

        if (timeInterval === 'hour' && (hours === 0 || hours === 12)) {
          return `${date.getMonth() + 1}/${date.getDate()} ${hours}:00`;
        } else if (timeInterval !== 'hour') {
          return `${date.getMonth() + 1}/${date.getDate()}`;
        }
        return '';
      } catch (error) {
        console.error('Error formatting date:', error, interval);
        return '';
      }
    },
    interval: 0,
    minTickGap: 10,
    padding: { left: 30, right: 30 }
  }),
  
  // Y轴配置
  yAxisConfig: {
    axisLine: false,
    tickLine: false,
    tick: { fontSize: 12, fill: '#A3AED0' }
  }
};

// 创建通用下拉框搜索处理函数
const createSearchHandler = (setValue, setFiltered, options) => {
  return (e, { value }) => {
    setValue(value);
    if (value.trim() === '') {
      setFiltered(options);
    } else {
      const filtered = options.filter(item =>
        item.text.toLowerCase().includes(value.toLowerCase())
      );
      setFiltered(filtered);
    }
  };
};

// 组件: 无数据显示
const NoDataDisplay = ({ message }) => (
  <div style={styles.noDataContainer}>
    {message}
  </div>
);

// 组件: 加载指示器
const LoadingIndicator = ({ message }) => (
  <div style={styles.loadingContainer}>
    <div className="ui active centered inline loader"></div>
    <p style={{ marginTop: '10px', color: '#666' }}>{message}</p>
  </div>
);

// 组件: 使用统计表格
const UsageStatsTable = ({ data, t, showLoading = false, loadingMessage = '' }) => {
  if (showLoading) {
    return <LoadingIndicator message={loadingMessage} />;
  }
  
  return (
    <div style={{ overflowX: 'auto' }}>
      <table className="ui celled table">
        <thead>
          <tr>
            <th>{t('dashboard.tokenStats.username')}</th>
            <th>{t('dashboard.tokenStats.tokenName')}</th>
            <th>{t('dashboard.tokenStats.inputTokens')}</th>
            <th>{t('dashboard.tokenStats.outputTokens')}</th>
            <th>{t('dashboard.tokenStats.totalTokens')}</th>
            <th>{t('dashboard.tokenStats.requestCount')}</th>
            <th>{t('dashboard.tokenStats.lastUsed')}</th>
          </tr>
        </thead>
        <tbody>
          {data.length > 0 ? (
            data.map((item, index) => (
              <tr key={index}>
                <td>{item.username || t('dashboard.tokenStats.unknownUser')}</td>
                <td>{item.token_name || t('dashboard.tokenStats.unknownToken')}</td>
                <td>{item.prompt_tokens}</td>
                <td>{item.completion_tokens}</td>
                <td>{item.total_tokens}</td>
                <td>{item.request_count}</td>
                <td>{new Date(item.last_used_time * 1000).toLocaleString('zh-CN')}</td>
              </tr>
            ))
          ) : (
            <tr>
              <td colSpan="7" style={{ textAlign: 'center' }}>
                {t('dashboard.tokenStats.noData')}
              </td>
            </tr>
          )}
        </tbody>
        {data.length > 0 && (
          <tfoot>
            <tr>
              <th colSpan="2" style={{ textAlign: 'right' }}>{t('dashboard.tokenStats.total')}:</th>
              <th>{utils.calculateTotal(data, 'prompt_tokens')}</th>
              <th>{utils.calculateTotal(data, 'completion_tokens')}</th>
              <th>{utils.calculateTotal(data, 'total_tokens')}</th>
              <th>{utils.calculateTotal(data, 'request_count')}</th>
              <th></th>
            </tr>
          </tfoot>
        )}
      </table>
    </div>
  );
};

// 组件: Token使用明细表格
const TokenDetailTable = ({ data, t }) => (
  <div style={{ overflowX: 'auto', marginTop: '20px' }}>
    <table className="ui striped table">
      <thead>
        <tr>
          <th>{t('dashboard.tokenStats.username')}</th>
          <th>{t('dashboard.tokenStats.tokenName')}</th>
          <th>{t('dashboard.tokenStats.inputTokens')}</th>
          <th>{t('dashboard.tokenStats.outputTokens')}</th>
          <th>{t('dashboard.tokenStats.totalTokens')}</th>
          <th>{t('dashboard.tokenStats.requestCount')}</th>
          <th>{t('dashboard.tokenStats.lastUsed')}</th>
        </tr>
      </thead>
      <tbody>
        {data.length > 0 ? (
          data.map((item, index) => (
            <tr key={index}>
              <td>{item.username}</td>
              <td>{item.token_name}</td>
              <td>{(item.prompt_tokens || 0).toLocaleString()}</td>
              <td>{(item.completion_tokens || 0).toLocaleString()}</td>
              <td>{(item.total_tokens || 0).toLocaleString()}</td>
              <td>{(item.request_count || 0).toLocaleString()}</td>
              <td>{item.last_used_time ? new Date(item.last_used_time * 1000).toLocaleString('zh-CN') : ''}</td>
            </tr>
          ))
        ) : (
          <tr>
            <td colSpan="7" style={{ textAlign: 'center' }}>
              {t('dashboard.tokenStats.noData') || '暂无数据'}
            </td>
          </tr>
        )}
      </tbody>
      {data.length > 0 && (
        <tfoot>
          <tr>
            <th colSpan="2" style={{ textAlign: 'right' }}>{t('dashboard.tokenStats.total') || '总计'}:</th>
            <th>{utils.calculateTotal(data, 'prompt_tokens').toLocaleString()}</th>
            <th>{utils.calculateTotal(data, 'completion_tokens').toLocaleString()}</th>
            <th>{utils.calculateTotal(data, 'total_tokens').toLocaleString()}</th>
            <th>{utils.calculateTotal(data, 'request_count').toLocaleString()}</th>
            <th></th>
          </tr>
        </tfoot>
      )}
    </table>
  </div>
);

// Dashboard 主组件
const Dashboard = () => {
  const { t } = useTranslation();
  
  // 数据状态
  const [data, setData] = useState([]);
  const [summaryData, setSummaryData] = useState({
    todayRequests: 0,
    todayQuota: 0,
    todayTokens: 0,
  });
  const [usageStats, setUsageStats] = useState([]);
  const [tokenUsageData, setTokenUsageData] = useState([]);
  const [tokenUsageByNameData, setTokenUsageByNameData] = useState([]);
  const [tokenDetailData, setTokenDetailData] = useState([]);
  const [isLoading, setIsLoading] = useState(true);
  const [processedData, setProcessedData] = useState({ chartData: [], modelNames: [] });
  
  // UI状态
  const [timeInterval, setTimeInterval] = useState('day');
  
  // 用户和Token选择
  const [users, setUsers] = useState([]);
  const [tokensByNameOptions, setTokensByNameOptions] = useState([]);
  const [tokenModelOptions, setTokenModelOptions] = useState([]);
  const [filteredUsers, setFilteredUsers] = useState([]);
  const [filteredTokensByNameOptions, setFilteredTokensByNameOptions] = useState([]);
  const [filteredTokenModelOptions, setFilteredTokenModelOptions] = useState([]);
  
  // 搜索状态
  const [userSearchValue, setUserSearchValue] = useState('');
  const [tokenSearchValueByName, setTokenSearchValueByName] = useState('');
  const [tokenSearchValueModel, setTokenSearchValueModel] = useState('');
  
  // 表单状态
  const [formValues, setFormValues] = useState({
    username: '',
    userId: '',
    tokenName: '',
    startTimestamp: '',
    endTimestamp: '',
  });
  const [tokenUsageByNameForm, setTokenUsageByNameForm] = useState({
    username: '',
    userId: '',
    tokenName: '',
    startDate: '',
    endDate: ''
  });
  
  // 渠道选择
  const [channels, setChannels] = useState([]);
  const [filteredChannelIds, setFilteredChannelIds] = useState([]);
  const [selectedChannelIds, setSelectedChannelIds] = useState([]);
  const [channelSearchValue, setChannelSearchValue] = useState('');
  const [filteredChannels, setFilteredChannels] = useState([]);
  const [modelMappingValues, setModelMappingValues] = useState([]);
  const [isChannelsInit, setIsChannelsInit] = useState(false);

  // 创建搜索处理函数
  const handleUserSearch = createSearchHandler(setUserSearchValue, setFilteredUsers, users);
  const handleTokenSearchByName = createSearchHandler(setTokenSearchValueByName, setFilteredTokensByNameOptions, tokensByNameOptions);
  const handleTokenSearchModel = createSearchHandler(setTokenSearchValueModel, setFilteredTokenModelOptions, tokenModelOptions);
  const handleChannelSearch = createSearchHandler(setChannelSearchValue, setFilteredChannels, channels);

  // 初始化
  useEffect(() => {
    fetchChannels();
    fetchUsers();
  }, []);

  // 当筛选模型变化时加载数据
  useEffect(() => {
    if(isChannelsInit){
      fetchDashboardData();
      fetchAllUserUsageStats();
      fetchAllTokensFromLogs();
      
      const { startDate, endDate} = tokenUsageByNameForm;
      if (startDate && endDate) {
        handleTokenUsageByNameSubmit();
      }
      
      if (formValues.username && formValues.tokenName && formValues.startTimestamp && formValues.endTimestamp) {
        handleSubmit();
      }
    }
  }, [modelMappingValues]);

  // 处理Token使用数据
  useEffect(() => {
    const processed = processTokenUsageData();
    setProcessedData(processed);
  }, [tokenUsageData, timeInterval]);

  // 获取渠道列表
  const fetchChannels = async () => {
    await utils.fetchApi('/api/channel/search?keyword=', (channelData) => {
      // 格式化渠道数据
      const formattedChannels = channelData.map(channel => ({
        key: channel.id,
        text: channel.name || `Channel ${channel.id}`,
        value: channel.id,
        status: channel.status,
        model_mapping: channel.model_mapping,
        content: (
          <div style={{ color: channel.status === 1 ? '#000' : '#999' }}>
            <span style={{
              display: 'inline-block',
              width: '8px',
              height: '8px',
              borderRadius: '50%',
              backgroundColor: channel.status === 1 ? '#21ba45' : '#ff4d4f',
              marginRight: '8px'
            }}/>
            {channel.name || `Channel ${channel.id}`}
          </div>
        )
      }));
      
      setChannels(formattedChannels);
      setFilteredChannels(formattedChannels);
      
      // 获取所有非激活渠道ID
      const inactiveChannelIds = channelData
        .filter(channel => channel.status !== 1)
        .map(channel => channel.id);

      // 从 localStorage 获取保存的筛选设置
      let savedFilters = [];
      try {
        const saved = localStorage.getItem('dashboard_filtered_channels');
        if (saved) {
          savedFilters = JSON.parse(saved);
        }
      } catch (e) {
        console.error('Error parsing saved filters:', e);
      }

      // 合并保存的筛选和非激活渠道
      const initialFilters = Array.from(new Set([...inactiveChannelIds, ...savedFilters]));
      
      setFilteredChannelIds(initialFilters);
      setSelectedChannelIds(initialFilters);
      
      // 收集所有被过滤渠道的model_mapping中的值
      collectModelMappingValues(channelData, initialFilters);
      setIsChannelsInit(true);
    });
  };

  // 收集模型映射值
  const collectModelMappingValues = (channelData, filteredIds) => {
    const mappingValues = [];
    
    channelData.forEach(channel => {
      if (filteredIds.includes(channel.id) && channel.model_mapping) {
        try {
          const mapping = JSON.parse(channel.model_mapping);
          Object.values(mapping).forEach(value => {
            if (!mappingValues.includes(value)) {
              mappingValues.push(value);
            }
          });
        } catch (e) {
          console.error(`Error parsing model_mapping for channel ${channel.id}:`, e);
        }
      }
    });
    
    setModelMappingValues(mappingValues);
  };

  // 获取用户列表
  const fetchUsers = async () => {
    await utils.fetchApi('/api/user/', (userData) => {
      const formattedUsers = userData.map(user => ({
        key: user.id,
        text: user.username,
        value: user.id,
        content: (
          <div>
            <div>{user.username}</div>
            <div style={{ fontSize: '0.8em', color: '#666' }}>{user.id}</div>
          </div>
        )
      }));
      setUsers(formattedUsers);
      setFilteredUsers(formattedUsers);
    });
  };

  // 获取用户Token
  const fetchUserTokens = async (userId, formType = 'both') => {
    await utils.fetchApi(`/api/token/searchByUserId/?userId=${userId}`, (tokenData) => {
      const formattedTokens = tokenData.map(token => ({
        key: token.id,
        text: token.name || t('dashboard.form.unknownToken'),
        value: token.name || ''
      }));

      if (formType === 'byName' || formType === 'both') {
        setTokensByNameOptions(formattedTokens);
        setFilteredTokensByNameOptions(formattedTokens);
        setTokenSearchValueByName('');
      }

      if (formType === 'model' || formType === 'both') {
        setTokenModelOptions(formattedTokens);
        setFilteredTokenModelOptions(formattedTokens);
        setTokenSearchValueModel('');
      }
    });
  };

  // 获取所有Token
  const fetchAllTokensFromLogs = async () => {
    await utils.fetchApi('/api/token/searchByUserId/?userId=', (tokenData) => {
      const formattedTokens = tokenData.map(token => ({
        key: token.id,
        text: token.name || t('dashboard.form.unknownToken'),
        value: token.name || ''
      }));
      setTokensByNameOptions(formattedTokens);
      setFilteredTokensByNameOptions(formattedTokens);
      setTokenSearchValueByName('');
    });
  };

  // 获取仪表盘数据
  const fetchDashboardData = async () => {
    try {
      const url = utils.buildApiUrl('/api/user/dashboard', {}, modelMappingValues);
      const response = await axios.get(url);
      if (response.data.success) {
        const dashboardData = response.data.data || [];
        setData(dashboardData);
        calculateSummary(dashboardData);
      }
    } catch (error) {
      console.error('Failed to fetch dashboard data:', error);
      setData([]);
      calculateSummary([]);
    }
  };

  // 计算数据摘要
  const calculateSummary = (dashboardData) => {
    if (!Array.isArray(dashboardData) || dashboardData.length === 0) {
      setSummaryData({
        todayRequests: 0,
        todayQuota: 0,
        todayTokens: 0,
      });
      return;
    }

    const today = new Date().toISOString().split('T')[0];
    const todayData = dashboardData.filter((item) => item.Day === today);

    const summary = {
      todayRequests: todayData.reduce((sum, item) => sum + item.RequestCount, 0),
      todayQuota: todayData.reduce((sum, item) => sum + item.Quota, 0) / 1000000,
      todayTokens: todayData.reduce((sum, item) => sum + item.PromptTokens + item.CompletionTokens, 0),
    };

    setSummaryData(summary);
  };

  // 处理用户选择
  const handleUserSelect = (e, { value }) => {
    setFormValues({
      ...formValues,
      username: value,
      userId: value,
      tokenName: ''
    });
    fetchUserTokens(value, 'model');
  };

  // 处理Token选择
  const handleTokenSelect = (e, { value }) => {
    setFormValues({
      ...formValues,
      tokenName: value
    });
  };

  // 处理表单输入变化
  const handleInputChange = (e) => {
    const { name, value } = e.target;
    setFormValues({
      ...formValues,
      [name]: value,
    });
  };

  // 处理表单提交
  const handleSubmit = (e) => {
    if (e) {
      e.preventDefault();
    }

    if (!formValues.username || !formValues.tokenName || !formValues.startTimestamp || !formValues.endTimestamp) {
      alert(t('dashboard.form.validation.required'));
      return;
    }

    const startTimestamp = Math.floor(new Date(formValues.startTimestamp).getTime() / 1000);
    const endTimestamp = Math.floor(new Date(formValues.endTimestamp).getTime() / 1000);

    fetchTokenUsageData(startTimestamp, endTimestamp);

    if (formValues.userId && formValues.tokenName) {
      fetchUserTokenStats(formValues.userId, formValues.tokenName, startTimestamp, endTimestamp);
    }
  };

  // 获取Token使用数据
  const fetchTokenUsageData = async (startTimestamp, endTimestamp) => {
    const { userId, tokenName } = formValues;
    const params = { userId, tokenName, startTimestamp, endTimestamp };
    const url = utils.buildApiUrl('/api/log/model/usage', params, modelMappingValues);
    
    await utils.fetchApi(url, 
      (data) => {
        setTokenUsageData(data);
      },
      () => setTokenUsageData([])
    );
  };

  // 处理Token使用数据
  const processTokenUsageData = () => {
    if (!Array.isArray(tokenUsageData) || tokenUsageData.length === 0) {
      return { chartData: [], modelNames: [] };
    }

    const hourlyData = {};
    const modelNames = new Set();

    // 使用表单中选择的开始和结束时间
    const startDate = new Date(formValues.startTimestamp);
    const endDate = new Date(formValues.endTimestamp);

    // 生成所有时间点
    for (let d = new Date(startDate); d <= endDate; d.setDate(d.getDate() + 1)) {
      const currentDate = new Date(d);
      // 如果是按小时统计，生成当天的24个小时点
      if (timeInterval === 'hour') {
        for (let h = 0; h < 24; h++) {
          currentDate.setHours(h, 0, 0, 0);
          const hourKey = currentDate.toISOString().slice(0, 13);
          hourlyData[hourKey] = {
            interval: hourKey,
            displayTime: `${currentDate.getMonth() + 1}/${currentDate.getDate()} ${h}:00`
          };
        }
      } else {
        const dayKey = currentDate.toISOString().split('T')[0];
        hourlyData[dayKey] = {
          interval: dayKey,
          displayTime: `${currentDate.getMonth() + 1}/${currentDate.getDate()}`
        };
      }
    }

    // 处理数据 - 使用北京时间
    tokenUsageData.forEach(entry => {
      try {
        // 将created_at转换为北京时间
        const beijingDate = utils.convertToBeiJingTime(entry.created_at);
        const key = timeInterval === 'hour'
          ? beijingDate.toISOString().slice(0, 13)  // 按小时
          : beijingDate.toISOString().split('T')[0]; // 按天

        if (hourlyData[key]) {
          modelNames.add(entry.model_name);

          // 累加 token 使用量
          hourlyData[key][entry.model_name] = (hourlyData[key][entry.model_name] || 0) + entry.usage;

          // 累加请求次数
          const requestKey = `${entry.model_name}_count`;
          hourlyData[key][requestKey] = (hourlyData[key][requestKey] || 0) + entry.request_count;
        }
      } catch (error) {
        console.error('Error processing entry:', error, entry);
      }
    });

    const chartData = Object.values(hourlyData).sort((a, b) => a.interval.localeCompare(b.interval));

    return {
      chartData,
      modelNames: Array.from(modelNames)
    };
  };

  // 处理时间序列数据
  const processTimeSeriesData = () => {
    const dailyData = {};

    // 获取日期范围
    const dates = data.map((item) => item.Day);
    const maxDate = new Date();
    let minDate = dates.length > 0
      ? new Date(Math.min(...dates.map((d) => new Date(d))))
      : new Date();

    // 确保至少显示7天的数据
    const sevenDaysAgo = new Date();
    sevenDaysAgo.setDate(sevenDaysAgo.getDate() - 6);
    if (minDate > sevenDaysAgo) {
      minDate = sevenDaysAgo;
    }

    // 生成所有日期
    for (let d = new Date(minDate); d <= maxDate; d.setDate(d.getDate() + 1)) {
      const dateStr = d.toISOString().split('T')[0];
      dailyData[dateStr] = {
        date: dateStr,
        requests: 0,
        quota: 0,
        tokens: 0,
      };
    }

    // 填充实际数据
    data.forEach((item) => {
      if (item.Day && dailyData[item.Day]) {
        dailyData[item.Day].requests += item.RequestCount;
        dailyData[item.Day].quota += item.Quota / 1000000;
        dailyData[item.Day].tokens += item.PromptTokens + item.CompletionTokens;
      }
    });

    return Object.values(dailyData).sort((a, b) => a.date.localeCompare(b.date));
  };

  // 处理模型数据
  const processModelData = () => {
    const timeData = {};

    // 获取日期范围
    const dates = data.map((item) => item.Day);
    const maxDate = new Date();
    let minDate = dates.length > 0
      ? new Date(Math.min(...dates.map((d) => new Date(d))))
      : new Date();

    // 确保至少显示7天的数据
    const sevenDaysAgo = new Date();
    sevenDaysAgo.setDate(sevenDaysAgo.getDate() - 6);
    if (minDate > sevenDaysAgo) {
      minDate = sevenDaysAgo;
    }

    // 获取所有唯一的模型名称
    const models = [...new Set(data.map((item) => item.ModelName))];

    // 生成所有日期
    for (let d = new Date(minDate); d <= maxDate; d.setDate(d.getDate() + 1)) {
      const dateStr = d.toISOString().split('T')[0];
      timeData[dateStr] = {
        date: dateStr,
      };

      // 初始化所有模型的数据为0
      models.forEach((model) => {
        timeData[dateStr][model] = 0;
      });
    }

    // 填充实际数据
    data.forEach((item) => {
      if (item.Day && item.ModelName && timeData[item.Day]) {
        timeData[item.Day][item.ModelName] = (item.PromptTokens || 0) + (item.CompletionTokens || 0);
      }
    });

    return Object.values(timeData).sort((a, b) => a.date.localeCompare(b.date));
  };

  // 获取所有唯一的模型名称
  const getUniqueModels = () => {
    return [...new Set(data.map((item) => item.ModelName))];
  };

  // 处理渠道选择
  const handleChannelSelect = (e, { value }) => {
    setSelectedChannelIds(value);
  };

  // 应用渠道筛选
  const applyChannelFilters = () => {
    setFilteredChannelIds(selectedChannelIds);
    localStorage.setItem('dashboard_filtered_channels', JSON.stringify(selectedChannelIds));
    collectModelMappingValues(channels.map(c => ({
      id: c.value,
      model_mapping: c.model_mapping
    })), selectedChannelIds);
  };

  // 清除筛选条件
  const clearChannelFilters = () => {
    const inactiveChannelIds = channels
      .filter(channel => channel.status !== 1)
      .map(channel => channel.value);
      
    setSelectedChannelIds(inactiveChannelIds);
    setFilteredChannelIds(inactiveChannelIds);
    localStorage.setItem('dashboard_filtered_channels', JSON.stringify(inactiveChannelIds));
  };

  // 获取所有用户Token使用情况
  const fetchAllUserUsageStats = async () => {
    setIsLoading(true);
    try {
      const endDate = new Date();
      const startDate = new Date();
      startDate.setDate(startDate.getDate() - 7);
      
      const startTimestamp = Math.floor(startDate.getTime() / 1000);
      const endTimestamp = Math.floor(endDate.getTime() / 1000);
      
      const url = utils.buildApiUrl('/api/log/stats', { startTimestamp, endTimestamp }, modelMappingValues);
      
      await utils.fetchApi(url, 
        (data) => {
          const filteredStats = data.filter(item => item.username);
          setUsageStats(filteredStats);
        },
        () => setUsageStats([])
      );
    } finally {
      setIsLoading(false);
    }
  };

  // 获取特定用户的Token使用统计
  const fetchUserTokenStats = async (userId, tokenName, startTimestamp, endTimestamp) => {
    setIsLoading(true);
    try {
      const params = { userId, tokenName, startTimestamp, endTimestamp };
      const url = utils.buildApiUrl('/api/log/user/stats', params, modelMappingValues);
      
      await utils.fetchApi(url, 
        (data) => setUsageStats(data),
        () => setUsageStats([])
      );
    } finally {
      setIsLoading(false);
    }
  };

  // 获取按名称的Token使用数据
  const fetchTokenUsageByName = async (startDate, endDate, username, tokenName) => {
    const startTimestamp = Math.floor(new Date(startDate).setHours(0, 0, 0, 0) / 1000);
    const endDateObj = new Date(endDate);
    endDateObj.setHours(23, 59, 59, 999);
    const endTimestamp = Math.floor(endDateObj.getTime() / 1000);

    const params = { startTimestamp, endTimestamp };
    if (username) params.userId = username;
    if (tokenName) params.tokenName = tokenName;
    
    const url = utils.buildApiUrl('/api/log/token/usage-by-name', params, modelMappingValues);

    await utils.fetchApi(url, (rawData) => {
      // 过滤掉username为空的数据
      const filteredData = rawData.filter(item => item.username);

      // 为柱状图准备数据
      const tokenGroups = {};
      const userSet = new Set();

      // 收集所有用户名和token名称
      filteredData.forEach(item => {
        userSet.add(item.username);
        if (!tokenGroups[item.token_name]) {
          tokenGroups[item.token_name] = { token_name: item.token_name };
        }
      });

      // 将Set转换为数组
      const userArray = Array.from(userSet);

      // 为每个token初始化每个用户的使用量为0
      Object.keys(tokenGroups).forEach(tokenName => {
        userArray.forEach(username => {
          tokenGroups[tokenName][username] = 0;
        });
      });

      // 填充柱状图数据
      filteredData.forEach(item => {
        if (tokenGroups[item.token_name]) {
          tokenGroups[item.token_name][item.username] = parseInt(item.total_tokens) || 0;
          // 存储请求次数
          tokenGroups[item.token_name][`${item.username}_count`] = parseInt(item.request_count) || 0;
        }
      });

      setTokenUsageByNameData(Object.values(tokenGroups));
      setTokenDetailData(filteredData);
    });
  };

  // 处理表单提交
  const handleTokenUsageByNameSubmit = (e) => {
    if (e) {
      e.preventDefault();
    }
    
    const { startDate, endDate, username, tokenName } = tokenUsageByNameForm;

    if (!startDate || !endDate) {
      alert(t('dashboard.form.validation.dateRequired'));
      return;
    }

    fetchTokenUsageByName(startDate, endDate, username, tokenName);
  };

  // 处理用户选择变更
  const handleTokenUsageByNameUserSelect = (e, { value }) => {
    // 当value为null或undefined时，表示用户点击了清除按钮
    if (!value && value !== 0) {
      setTokenUsageByNameForm({
        ...tokenUsageByNameForm,
        username: '',
        userId: '',
        tokenName: ''
      });
      fetchAllTokensFromLogs();
      return;
    }

    // 正常选择用户的逻辑
    setTokenUsageByNameForm({
      ...tokenUsageByNameForm,
      username: value,
      userId: value,
      tokenName: ''
    });
    fetchUserTokens(value, 'byName');
  };

  // 计算图表数据
  const timeSeriesData = processTimeSeriesData();
  const modelData = processModelData();
  const models = getUniqueModels();
  const { chartData, modelNames } = processedData;

  // 渲染图表、表单和表格
  return (
    <div className='dashboard-container'>
      {/* 渠道筛选组件 */}
      <Card fluid className='chart-card'>
        <Card.Content>
          <Card.Header>{t('dashboard.charts.channelFilter.title')}</Card.Header>
          <Form style={{ marginTop: '0.5rem' }}>
            <Form.Group widths='equal'>
              <Form.Field>
                <label>{t('dashboard.charts.channelFilter.channels_to_exclude')}</label>
                <Dropdown
                  placeholder={t('dashboard.charts.channelFilter.select_channels')}
                  fluid
                  multiple
                  search
                  selection
                  options={filteredChannels}
                  value={selectedChannelIds}
                  onChange={handleChannelSelect}
                  onSearchChange={handleChannelSearch}
                  searchQuery={channelSearchValue}
                  renderLabel={(item) => ({
                    color: item.status === 1 ? 'green' : 'red',
                    content: item.text
                  })}
                />
              </Form.Field>
            </Form.Group>
            <div style={{ display: 'flex', justifyContent: 'space-between', marginTop: '0.5rem' }}>
              <div>
                {filteredChannelIds.length > 0 ? 
                  <small style={{ color: '#666' }}>
                    {t('dashboard.charts.channelFilter.currently_excluding')} {filteredChannelIds.length} {t('dashboard.charts.channelFilter.channels') || '个渠道'}
                  </small> : null
                }
              </div>
              <div>
                <Button type='button' size='small' onClick={clearChannelFilters}>
                  {t('dashboard.charts.channelFilter.clear_filters')}
                </Button>
                <Button type='button' size='small' primary onClick={applyChannelFilters}>
                  {t('dashboard.charts.channelFilter.apply_filters')}
                </Button>
              </div>
            </div>
          </Form>
        </Card.Content>
      </Card>

      {/* 概览统计图表 */}
      <Grid columns={3} stackable className='charts-grid'>
        {/* 请求数统计 */}
        <Grid.Column>
          <Card fluid className='chart-card'>
            <Card.Content>
              <Card.Header>{t('dashboard.charts.requests.title')}</Card.Header>
              <div className='chart-container'>
                <ResponsiveContainer width='100%' height={120} margin={{ left: 10, right: 10 }}>
                  <LineChart data={timeSeriesData}>
                    <CartesianGrid
                      strokeDasharray='3 3'
                      vertical={chartConfig.lineChart.grid.vertical}
                      horizontal={chartConfig.lineChart.grid.horizontal}
                      opacity={chartConfig.lineChart.grid.opacity}
                    />
                    <XAxis {...chartConfigs.xAxisConfig} />
                    <YAxis hide={true} />
                    <Tooltip
                      contentStyle={styles.tooltipContent}
                      formatter={(value) => [value, t('dashboard.charts.requests.tooltip')]}
                      labelFormatter={(label) => `${t('dashboard.statistics.tooltip.date')}: ${utils.formatDate(label)}`}
                    />
                    <Line
                      type='monotone'
                      dataKey='requests'
                      stroke={chartConfig.colors.requests}
                      strokeWidth={chartConfig.lineChart.line.strokeWidth}
                      dot={chartConfig.lineChart.line.dot}
                      activeDot={chartConfig.lineChart.line.activeDot}
                    />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            </Card.Content>
          </Card>
        </Grid.Column>

        {/* 配额统计 */}
        <Grid.Column>
          <Card fluid className='chart-card'>
            <Card.Content>
              <Card.Header>{t('dashboard.charts.quota.title')}</Card.Header>
              <div className='chart-container'>
                <ResponsiveContainer width='100%' height={120} margin={{ left: 10, right: 10 }}>
                  <LineChart data={timeSeriesData}>
                    <CartesianGrid
                      strokeDasharray='3 3'
                      vertical={chartConfig.lineChart.grid.vertical}
                      horizontal={chartConfig.lineChart.grid.horizontal}
                      opacity={chartConfig.lineChart.grid.opacity}
                    />
                    <XAxis {...chartConfigs.xAxisConfig} />
                    <YAxis hide={true} />
                    <Tooltip
                      contentStyle={styles.tooltipContent}
                      formatter={(value) => [value.toFixed(6), t('dashboard.charts.quota.tooltip')]}
                      labelFormatter={(label) => `${t('dashboard.statistics.tooltip.date')}: ${utils.formatDate(label)}`}
                    />
                    <Line
                      type='monotone'
                      dataKey='quota'
                      stroke={chartConfig.colors.quota}
                      strokeWidth={chartConfig.lineChart.line.strokeWidth}
                      dot={chartConfig.lineChart.line.dot}
                      activeDot={chartConfig.lineChart.line.activeDot}
                    />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            </Card.Content>
          </Card>
        </Grid.Column>

        {/* Token统计 */}
        <Grid.Column>
          <Card fluid className='chart-card'>
            <Card.Content>
              <Card.Header>{t('dashboard.charts.tokens.title')}</Card.Header>
              <div className='chart-container'>
                <ResponsiveContainer width='100%' height={120} margin={{ left: 10, right: 10 }}>
                  <LineChart data={timeSeriesData}>
                    <CartesianGrid
                      strokeDasharray='3 3'
                      vertical={chartConfig.lineChart.grid.vertical}
                      horizontal={chartConfig.lineChart.grid.horizontal}
                      opacity={chartConfig.lineChart.grid.opacity}
                    />
                    <XAxis {...chartConfigs.xAxisConfig} />
                    <YAxis hide={true} />
                    <Tooltip
                      contentStyle={styles.tooltipContent}
                      formatter={(value) => [value, t('dashboard.charts.tokens.tooltip')]}
                      labelFormatter={(label) => `${t('dashboard.statistics.tooltip.date')}: ${utils.formatDate(label)}`}
                    />
                    <Line
                      type='monotone'
                      dataKey='tokens'
                      stroke={chartConfig.colors.tokens}
                      strokeWidth={chartConfig.lineChart.line.strokeWidth}
                      dot={chartConfig.lineChart.line.dot}
                      activeDot={chartConfig.lineChart.line.activeDot}
                    />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            </Card.Content>
          </Card>
        </Grid.Column>
      </Grid>

      {/* 模型使用统计 */}
      <Card fluid className='chart-card'>
        <Card.Content>
          <Card.Header>{t('dashboard.statistics.title')}</Card.Header>
          <div className='chart-container'>
            <ResponsiveContainer width='100%' height={300}>
              <BarChart data={modelData}>
                <CartesianGrid strokeDasharray='3 3' vertical={false} opacity={0.1} />
                <XAxis {...chartConfigs.xAxisConfig} />
                <YAxis {...chartConfigs.yAxisConfig} />
                <Tooltip
                  contentStyle={styles.tooltipContent}
                  labelFormatter={(label) => {
                    const comDate = label + ':00:00';
                    const date = new Date(comDate);
                    return `日期: ${date.getMonth() + 1}/${date.getDate()}`;
                  }}
                />
                <Legend wrapperStyle={{ paddingTop: '20px' }} />
                {models.map((model, index) => (
                  <Bar
                    key={model}
                    dataKey={model}
                    stackId='a'
                    fill={utils.getRandomColor(index)}
                    name={model}
                    radius={[4, 4, 0, 0]}
                  />
                ))}
              </BarChart>
            </ResponsiveContainer>
          </div>
        </Card.Content>
      </Card>

      {/* Token Usage By Name Form */}
      <Grid.Column>
        <Card fluid className='chart-card'>
          <Card.Content>
            <Card.Header>{t('dashboard.tokenUsageByName.formTitle')}</Card.Header>
            <Form onSubmit={handleTokenUsageByNameSubmit}>
              <Form.Group widths='equal'>
                <Form.Field>
                  <label>{t('dashboard.form.username')}</label>
                  <Dropdown
                    placeholder={t('dashboard.form.username')}
                    fluid
                    search
                    selection
                    clearable
                    options={filteredUsers}
                    value={tokenUsageByNameForm.username}
                    onChange={handleTokenUsageByNameUserSelect}
                    onSearchChange={handleUserSearch}
                    searchQuery={userSearchValue}
                  />
                </Form.Field>
                <Form.Field>
                  <label>{t('dashboard.form.tokenName')}</label>
                  <Dropdown
                    placeholder={t('dashboard.form.tokenName')}
                    fluid
                    search
                    selection
                    clearable
                    options={filteredTokensByNameOptions}
                    value={tokenUsageByNameForm.tokenName}
                    onChange={(e, { value }) => setTokenUsageByNameForm({
                      ...tokenUsageByNameForm,
                      tokenName: value
                    })}
                    onSearchChange={handleTokenSearchByName}
                    searchQuery={tokenSearchValueByName}
                  />
                </Form.Field>
                <Form.Field>
                  <label>{t('dashboard.form.startTimestamp')}</label>
                  <input
                    type='date'
                    value={tokenUsageByNameForm.startDate}
                    onChange={(e) => setTokenUsageByNameForm({
                      ...tokenUsageByNameForm,
                      startDate: e.target.value
                    })}
                    required
                  />
                </Form.Field>
                <Form.Field>
                  <label>{t('dashboard.form.endTimestamp')}</label>
                  <input
                    type='date'
                    value={tokenUsageByNameForm.endDate}
                    onChange={(e) => setTokenUsageByNameForm({
                      ...tokenUsageByNameForm,
                      endDate: e.target.value
                    })}
                    required
                  />
                </Form.Field>
                <Form.Field>
                  <label>&nbsp;</label>
                  <Button fluid type='submit'>{t('dashboard.form.submit')}</Button>
                </Form.Field>
              </Form.Group>
            </Form>
          </Card.Content>
        </Card>
      </Grid.Column>

      {/* Token Usage By Name Chart */}
      <Grid.Column width={16}>
        <Card fluid className='chart-card'>
          <Card.Content>
            <Card.Header style={styles.tableHeader}>{t('dashboard.tokenUsageByName.chartTitle')}</Card.Header>
            <div className='chart-container'>
              {tokenUsageByNameData.length > 0 ? (
                <ResponsiveContainer width='100%' height={450}>
                  <BarChart
                    data={tokenUsageByNameData}
                    margin={{ top: 20, right: 30, left: 20, bottom: 100 }}
                    barSize={35}
                  >
                    <CartesianGrid strokeDasharray='3 3' vertical={false} opacity={0.1} />
                    <XAxis
                      dataKey="token_name"
                      axisLine={false}
                      tickLine={false}
                      tick={{ fontSize: 12, fill: '#555' }}
                      interval={0}
                      angle={45}
                      textAnchor="start"
                      height={70}
                      padding={{ left: 10, right: 10 }}
                    />
                    <YAxis {...chartConfigs.yAxisConfig} />
                    <Tooltip
                      contentStyle={styles.tooltipContent}
                      formatter={(value, name, props) => {
                        const displayName = name === 'unknown' ? t('dashboard.tokenStats.unknownUser') : name;
                        const requestCount = props.payload[`${name}_count`] || 0;
                        return [
                          <div style={{
                            display: 'flex',
                            justifyContent: 'space-between',
                            width: '100%',
                            minWidth: '300px'
                          }}>
                            <span>{value} {t('dashboard.tokenStats.tokens')}</span>
                            <span style={{ color: '#888', marginLeft: '10px' }}>{t('dashboard.tokenStats.usageCount')}: {requestCount}</span>
                          </div>,
                          displayName
                        ];
                      }}
                    />
                    <Legend
                      formatter={(value) => value === 'unknown' ? t('dashboard.tokenStats.unknownUser') : value}
                      height={36}
                      verticalAlign="top"
                      iconSize={10}
                      iconType="circle"
                      wrapperStyle={{
                        paddingTop: '10px',
                        fontSize: '12px',
                        lineHeight: '20px'
                      }}
                    />
                    {tokenUsageByNameData.length > 0 &&
                      Object.keys(tokenUsageByNameData[0])
                        .filter(key => key !== 'token_name' && !key.endsWith('_count'))
                        .map((username, index) => (
                          <Bar
                            key={username}
                            dataKey={username}
                            stackId="a"
                            fill={utils.getRandomColor(index)}
                            name={username}
                            radius={[4, 4, 0, 0]}
                          />
                        ))
                    }
                  </BarChart>
                </ResponsiveContainer>
              ) : (
                <NoDataDisplay message={t('dashboard.tokenUsageByName.noDataHint')} />
              )}
            </div>
          </Card.Content>
        </Card>
      </Grid.Column>

      {/* Token使用明细统计表 */}
      <Grid.Column width={16}>
        <Card fluid className='chart-card'>
          <Card.Content>
            <Card.Header style={styles.tableHeader}>
              {t('dashboard.tokenUsageByName.tableTitle') || 'Token使用明细统计表'}
            </Card.Header>
            <TokenDetailTable data={tokenDetailData} t={t} />
          </Card.Content>
        </Card>
      </Grid.Column>

      {/* Token Model Usage Chart Form */}
      <Grid.Column>
        <Card fluid className='chart-card'>
          <Card.Content>
            <Card.Header>{t('dashboard.tokenModelUsage.formtitle')}</Card.Header>
            <Form onSubmit={handleSubmit}>
              <Form.Group widths='equal'>
                <Form.Field>
                  <label>{t('dashboard.form.username')}</label>
                  <Dropdown
                    placeholder={t('dashboard.form.username')}
                    fluid
                    search
                    selection
                    options={filteredUsers}
                    value={formValues.username}
                    onChange={handleUserSelect}
                    onSearchChange={handleUserSearch}
                    searchQuery={userSearchValue}
                  />
                </Form.Field>
                <Form.Field>
                  <label>{t('dashboard.form.tokenName')}</label>
                  <Dropdown
                    placeholder={t('dashboard.form.tokenName')}
                    fluid
                    search
                    selection
                    options={filteredTokenModelOptions}
                    value={formValues.tokenName}
                    onChange={handleTokenSelect}
                    onSearchChange={handleTokenSearchModel}
                    searchQuery={tokenSearchValueModel}
                  />
                </Form.Field>
                <Form.Field>
                  <label>{t('dashboard.form.startTimestamp')}</label>
                  <input
                    type='datetime-local'
                    name='startTimestamp'
                    value={formValues.startTimestamp}
                    onChange={handleInputChange}
                    required
                  />
                </Form.Field>
                <Form.Field>
                  <label>{t('dashboard.form.endTimestamp')}</label>
                  <input
                    type='datetime-local'
                    name='endTimestamp'
                    value={formValues.endTimestamp}
                    onChange={handleInputChange}
                    required
                  />
                </Form.Field>
                <Form.Field>
                  <label>&nbsp;</label>
                  <Button fluid type='submit'>{t('dashboard.form.submit')}</Button>
                </Form.Field>
              </Form.Group>
            </Form>
          </Card.Content>
        </Card>
      </Grid.Column>

      {/* Token Model Usage Chart */}
      <Grid.Column width={16}>
        <Card fluid className='chart-card'>
          <Card.Content>
            <Card.Header style={styles.tableHeader}>{t('dashboard.tokenModelUsage.chartTitle')}</Card.Header>
            <div style={{
              display: 'flex',
              justifyContent: 'flex-end',
              marginTop: '10px',
              marginBottom: '10px'
            }}>
              <select
                value={timeInterval}
                onChange={(e) => setTimeInterval(e.target.value)}
                style={{
                  border: '1px solid #e0e1e2',
                  borderRadius: '4px',
                  padding: '6px 12px',
                  fontSize: '13px',
                  color: '#555',
                  background: '#ffffff',
                  cursor: 'pointer',
                  outline: 'none',
                  width: '120px',
                  height: '34px',
                  boxShadow: '0 1px 2px rgba(0,0,0,0.05)',
                  appearance: 'none',
                  backgroundImage: 'url("data:image/svg+xml;charset=US-ASCII,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20width%3D%22292.4%22%20height%3D%22292.4%22%3E%3Cpath%20fill%3D%22%23555%22%20d%3D%22M287%2069.4a17.6%2017.6%200%200%200-13-5.4H18.4c-5%200-9.3%201.8-12.9%205.4A17.6%2017.6%200%200%200%200%2082.2c0%205%201.8%209.3%205.4%2012.9l128%20127.9c3.6%203.6%207.8%205.4%2012.8%205.4s9.2-1.8%2012.8-5.4L287%2095c3.5-3.5%205.4-7.8%205.4-12.8%200-5-1.9-9.2-5.5-12.8z%22%2F%3E%3C%2Fsvg%3E")',
                  backgroundRepeat: 'no-repeat',
                  backgroundPosition: 'right 8px center',
                  backgroundSize: '10px 6px',
                  paddingRight: '24px',
                  transition: 'all 0.2s ease',
                  fontWeight: '500'
                }}
              >
                <option value='day'>{t('dashboard.form.timeInterval.day')}</option>
                <option value='hour'>{t('dashboard.form.timeInterval.hour')}</option>
              </select>
            </div>
            <div className='chart-container'>
              {chartData.length > 0 ? (
                <ResponsiveContainer width='100%' height={500}>
                  <BarChart
                    data={chartData}
                    barSize={timeInterval === 'hour' ? 8 : 60}
                    margin={{ top: 20, right: 30, left: 20, bottom: 60 }}
                  >
                    <CartesianGrid strokeDasharray='3 3' vertical={false} opacity={0.1} />
                    <XAxis {...chartConfigs.xAxisIntervalConfig(timeInterval)} />
                    <YAxis
                      axisLine={false}
                      tickLine={false}
                      tick={{ fontSize: 14, fill: '#A3AED0' }}
                      width={60}
                    />
                    <Tooltip
                      contentStyle={styles.tooltipContent}
                      formatter={(value, name, props) => {
                        const displayName = name === 'unknown' ? t('dashboard.tokenStats.unknownUser') : name;
                        const requestCount = props.payload[`${name}_count`] || 0;
                        return [
                          <div style={{
                            display: 'flex',
                            justifyContent: 'space-between',
                            width: '100%',
                            minWidth: '300px'
                          }}>
                            <span>{value} {t('dashboard.tokenStats.tokens')}</span>
                            <span style={{ color: '#888', marginLeft: '10px' }}>{t('dashboard.tokenStats.usageCount')}: {requestCount}</span>
                          </div>,
                          displayName
                        ];
                      }}
                      itemSorter={(item) => -item.value}
                      labelFormatter={(label) => {
                        const comDate = label + ':00:00';
                        const date = new Date(comDate);

                        if (timeInterval === 'hour') {
                          return `日期: ${date.getMonth() + 1}/${date.getDate()} ${date.getHours()}:00`;
                        } else {
                          return `日期: ${date.getMonth() + 1}/${date.getDate()}`;
                        }
                      }}
                      wrapperStyle={{ zIndex: 1000 }}
                    />
                    <Legend
                      formatter={(value) => value === 'unknown' ? t('dashboard.tokenStats.unknownUser') : value}
                      height={36}
                      verticalAlign="top"
                      iconSize={10}
                      iconType="circle"
                      wrapperStyle={{
                        paddingTop: '10px',
                        fontSize: '12px',
                        lineHeight: '20px'
                      }}
                    />
                    {modelNames.map((modelName, index) => (
                      <Bar
                        key={modelName}
                        dataKey={modelName}
                        stackId="a"
                        fill={utils.getRandomColor(index)}
                        name={modelName}
                        radius={[4, 4, 0, 0]}
                      />
                    ))}
                  </BarChart>
                </ResponsiveContainer>
              ) : (
                <NoDataDisplay message={t('dashboard.tokenModelUsage.noDataHint')} />
              )}
            </div>
          </Card.Content>
        </Card>
      </Grid.Column>

      {/* 用户使用统计表格 */}
      <Grid.Column width={16}>
        <Card fluid className='chart-card'>
          <Card.Content>
            <Card.Header style={styles.tableHeader}>
              {t('dashboard.tokenModelUsage.tableTitle')}
            </Card.Header>
            <UsageStatsTable 
              data={usageStats} 
              t={t} 
              showLoading={isLoading} 
              loadingMessage={t('dashboard.tokenStats.loading')} 
            />
          </Card.Content>
        </Card>
      </Grid.Column>
    </div>
  );
};

export default Dashboard;
