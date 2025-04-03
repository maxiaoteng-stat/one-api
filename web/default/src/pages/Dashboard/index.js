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

// 时区转换辅助函数
const convertToBeiJingTime = (dateStr) => {
  // 如果是字符串中包含空格，先替换为T
  const normalizedDateStr = dateStr.replace(' ', 'T');
  // 创建Date对象
  const date = new Date(normalizedDateStr);
  // 返回一个新的Date对象，调整为北京时间
  return new Date(date.getTime() + 8 * 60 * 60 * 1000);
};

const Dashboard = () => {
  const { t } = useTranslation();
  const [data, setData] = useState([]);
  const [summaryData, setSummaryData] = useState({
    todayRequests: 0,
    todayQuota: 0,
    todayTokens: 0,
  });
  const [users, setUsers] = useState([]);
  const [tokensByNameOptions, setTokensByNameOptions] = useState([]);
  const [tokenModelOptions, setTokenModelOptions] = useState([]);
  const [filteredUsers, setFilteredUsers] = useState([]);
  const [filteredTokensByNameOptions, setFilteredTokensByNameOptions] = useState([]);
  const [filteredTokenModelOptions, setFilteredTokenModelOptions] = useState([]);
  const [userSearchValue, setUserSearchValue] = useState('');
  const [tokenSearchValueByName, setTokenSearchValueByName] = useState('');
  const [tokenSearchValueModel, setTokenSearchValueModel] = useState('');
  const [formValues, setFormValues] = useState({
    username: '',
    userId: '',
    tokenName: '',
    startTimestamp: '',
    endTimestamp: '',
  });
  const [timeInterval, setTimeInterval] = useState('day'); // 默认为天
  const [usageStats, setUsageStats] = useState([]);
  const [isLoading, setIsLoading] = useState(true);
  const [tokenUsageByNameData, setTokenUsageByNameData] = useState([]);
  const [tokenUsageByNameForm, setTokenUsageByNameForm] = useState({
    username: '',
    userId: '',
    tokenName: '',
    startDate: '',
    endDate: ''
  });

  useEffect(() => {
    fetchDashboardData();
    fetchUsers();
    fetchAllUserUsageStats();
    fetchAllTokensFromLogs(); // 初始加载所有token
  }, []);

  const handleUserSearch = (e, { value }) => {
    setUserSearchValue(value);
    if (value.trim() === '') {
      setFilteredUsers(users);
    } else {
      const filtered = users.filter(user => 
        user.text.toLowerCase().includes(value.toLowerCase())
      );
      setFilteredUsers(filtered);
    }
  };

  const handleTokenSearchByName = (e, { value }) => {
    setTokenSearchValueByName(value);
    if (value.trim() === '') {
      setFilteredTokensByNameOptions(tokensByNameOptions);
    } else {
      const filtered = tokensByNameOptions.filter(token => 
        token.text.toLowerCase().includes(value.toLowerCase())
      );
      setFilteredTokensByNameOptions(filtered);
    }
  };

  const handleTokenSearchModel = (e, { value }) => {
    setTokenSearchValueModel(value);
    if (value.trim() === '') {
      setFilteredTokenModelOptions(tokenModelOptions);
    } else {
      const filtered = tokenModelOptions.filter(token => 
        token.text.toLowerCase().includes(value.toLowerCase())
      );
      setFilteredTokenModelOptions(filtered);
    }
  };

  const handleUserSelect = (e, { value }) => {
    setFormValues({
      ...formValues,
      username: value,
      userId: value,
      tokenName: '' // 重置 token 选择
    });
    
    // 加载该用户的 token 到Model表单
    fetchUserTokens(value, 'model');
  };

  const handleTokenSelect = (e, { value }) => {
    setFormValues({
      ...formValues,
      tokenName: value
    });
  };

  const handleInputChange = (e) => {
    const { name, value } = e.target;
    setFormValues({
      ...formValues,
      [name]: value,
    });
  };

  const handleSubmit = (e) => {
    e.preventDefault();
    
    // 验证所有必填字段
    if (!formValues.username || !formValues.tokenName || !formValues.startTimestamp || !formValues.endTimestamp) {
      // 使用 Semantic UI 的 Message 组件或其他提示方式
      alert(t('dashboard.form.validation.required')); // 或者使用其他更友好的提示方式
      return;
    }

    const startTimestamp = Math.floor(new Date(formValues.startTimestamp).getTime() / 1000);
    const endTimestamp = Math.floor(new Date(formValues.endTimestamp).getTime() / 1000);
    
    fetchTokenUsageData(startTimestamp, endTimestamp);
    
    if (formValues.userId && formValues.tokenName) {
      fetchUserTokenStats(formValues.userId, formValues.tokenName, startTimestamp, endTimestamp);
    }
  };

  const [tokenUsageData, setTokenUsageData] = useState({});

  const fetchTokenUsageData = async (startTimestamp, endTimestamp) => {
    console.info("entry fetchTokenUsageData ~~~~~");
    const { userId, tokenName } = formValues;
    try {
      const res = await API.get(`/api/log/model/usage?userId=${userId}&tokenName=${tokenName}&startTimestamp=${startTimestamp}&endTimestamp=${endTimestamp}`);
      const { success, data } = res.data;
      if (success) {
        const normalizedData = Array.isArray(data) ? data : [];
        setTokenUsageData(normalizedData);
        console.log('Fetched Token Usage Data:', normalizedData);
      } else {
        console.error('API request was not successful:', res.data);
        setTokenUsageData([]);
      }
    } catch (error) {
      console.error('Failed to fetch token usage data:', error);
      setTokenUsageData([]);
    }
  };

  const processTokenUsageData = () => {
    if (!Array.isArray(tokenUsageData)) {
      console.log('tokenUsageData is not an array:', tokenUsageData);
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
        const beijingDate = convertToBeiJingTime(entry.created_at);
        const key = timeInterval === 'hour'
          ? beijingDate.toISOString().slice(0, 13)  // 按小时
          : beijingDate.toISOString().split('T')[0]; // 按天

        if (hourlyData[key]) {
          modelNames.add(entry.model_name);
          
          // 累加 token 使用量
          hourlyData[key][entry.model_name] = (hourlyData[key][entry.model_name] || 0) + entry.usage;
          
          // 累加请求次数 (新增)
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

  const [processedData, setProcessedData] = useState({ chartData: [], modelNames: [] });

  useEffect(() => {
    const processed = processTokenUsageData();
    setProcessedData(processed);
  }, [tokenUsageData, timeInterval]);

  const { chartData, modelNames } = processedData;

  const fetchDashboardData = async () => {
    try {
      const response = await axios.get('/api/user/dashboard');
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
      todayRequests: todayData.reduce(
        (sum, item) => sum + item.RequestCount,
        0
      ),
      todayQuota:
        todayData.reduce((sum, item) => sum + item.Quota, 0) / 1000000,
      todayTokens: todayData.reduce(
        (sum, item) => sum + item.PromptTokens + item.CompletionTokens,
        0
      ),
    };

    setSummaryData(summary);
  };

  // 处理数据以供折线图使用，补充缺失的日期
  const processTimeSeriesData = () => {
    const dailyData = {};

    // 获取日期范围
    const dates = data.map((item) => item.Day);
    const maxDate = new Date(); // 总是使用今天作为最后一天
    let minDate =
      dates.length > 0
        ? new Date(Math.min(...dates.map((d) => new Date(d))))
        : new Date();

    // 确保至少显示7天的数据
    const sevenDaysAgo = new Date();
    sevenDaysAgo.setDate(sevenDaysAgo.getDate() - 6); // -6是因为包含今天
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
      // 检查item.Day是否存在且在dailyData中有对应的项
      if (item.Day && dailyData[item.Day]) {
        dailyData[item.Day].requests += item.RequestCount;
        dailyData[item.Day].quota += item.Quota / 1000000;
        dailyData[item.Day].tokens += item.PromptTokens + item.CompletionTokens;
      } else {
        console.log('Missing date in dailyData:', item.Day, item);
      }
    });

    return Object.values(dailyData).sort((a, b) =>
      a.date.localeCompare(b.date)
    );
  };

  // 处理数据以供堆叠柱状图使用
  const processModelData = () => {
    const timeData = {};

    // 获取日期范围
    const dates = data.map((item) => item.Day);
    const maxDate = new Date(); // 总是使用今天作为最后一天
    let minDate =
      dates.length > 0
        ? new Date(Math.min(...dates.map((d) => new Date(d))))
        : new Date();

    // 确保至少显示7天的数据
    const sevenDaysAgo = new Date();
    sevenDaysAgo.setDate(sevenDaysAgo.getDate() - 6); // -6是因为包含今天
    if (minDate > sevenDaysAgo) {
      minDate = sevenDaysAgo;
    }

    // 生成所有日期
    for (let d = new Date(minDate); d <= maxDate; d.setDate(d.getDate() + 1)) {
      const dateStr = d.toISOString().split('T')[0];
      timeData[dateStr] = {
        date: dateStr,
      };

      // 初始化所有模型的数据为0
      const models = [...new Set(data.map((item) => item.ModelName))];
      models.forEach((model) => {
        timeData[dateStr][model] = 0;
      });
    }

    // 填充实际数据
    data.forEach((item) => {
      // 检查item.Day和item.ModelName是否都存在，且item.Day在timeData中有对应项
      if (item.Day && item.ModelName && timeData[item.Day]) {
        // 确保模型名称可以作为对象属性使用
        timeData[item.Day][item.ModelName] = 
          (item.PromptTokens || 0) + (item.CompletionTokens || 0);
      } else {
        console.warn('Invalid data item:', item);
      }
    });

    return Object.values(timeData).sort((a, b) => a.date.localeCompare(b.date));
  };

  // 获取所有唯一的模型名称
  const getUniqueModels = () => {
    return [...new Set(data.map((item) => item.ModelName))];
  };

  const timeSeriesData = processTimeSeriesData();
  const modelData = processModelData();
  const models = getUniqueModels();

  // 生成随机颜色
  const getRandomColor = (index) => {
    return chartConfig.barColors[index % chartConfig.barColors.length];
  };

  // 修改日期格式化函数，确保显示北京时间
  const formatDate = (dateStr) => {
    const date = new Date(dateStr);
    // 这里不需要再加8小时，因为只是格式化显示
    return date.toLocaleDateString('zh-CN', {
      month: 'numeric',
      day: 'numeric',
    });
  };

  // 修改所有 XAxis 配置
  const xAxisConfig = {
    dataKey: 'date',
    axisLine: false,
    tickLine: false,
    tick: {
      fontSize: 12,
      fill: '#A3AED0',
      textAnchor: 'middle', // 文本居中对齐
    },
    tickFormatter: formatDate,
    interval: 0,
    minTickGap: 5,
    padding: { left: 30, right: 30 }, // 增加两侧的内边距，确保首尾标签完整显示
  };

  // 修改 XAxis 配置以适应小时或天数据
  const xAxisIntervalConfig = {
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

        // 使用北京时间显示
        if (timeInterval === 'hour' && (hours === 0 || hours === 12)) {
          return `${date.getMonth() + 1}/${date.getDate()} ${hours}:00`;
        } else if (timeInterval !== 'hour') {
          return `${date.getMonth() + 1}/${date.getDate()}`;
        }
        return ''; // 不显示其他时间点
      } catch (error) {
        console.error('Error formatting date:', error, interval);
        return '';
      }
    },
    interval: 0,
    minTickGap: 10,
    padding: { left: 30, right: 30 }
  };

  // 获取用户列表函数
  const fetchUsers = async () => {
    try {
      const response = await API.get('/api/user/');
      if (response.data.success) {
        const userData = response.data.data || [];
        // 转换数据格式以适应下拉框
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
      }
    } catch (error) {
      console.error('Failed to fetch users:', error);
    }
  };

  // 新增函数：获取特定用户的 token 列表
  const fetchUserTokens = async (userId, formType = 'both') => {
    try {
      const response = await API.get(`/api/token/searchByUserId/?userId=${userId}`);
      if (response.data.success) {
        const tokenData = response.data.data || [];
        // 转换数据格式以适应下拉框
        const formattedTokens = tokenData.map(token => ({
          key: token.id,
          text: token.name || t('dashboard.form.unknownToken'),
          value: token.name || ''
        }));
        
        // 根据表单类型更新对应的tokens状态
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
      }
    } catch (error) {
      console.error('Failed to fetch user tokens:', error);
      if (formType === 'byName' || formType === 'both') {
        setTokensByNameOptions([]);
        setFilteredTokensByNameOptions([]);
      }
      
      if (formType === 'model' || formType === 'both') {
        setTokenModelOptions([]);
        setFilteredTokenModelOptions([]);
      }
    }
  };

  // 新增函数：获取所有token名称
  const fetchAllTokensFromLogs = async () => {
    try {
      const response = await API.get('/api/token/searchByUserId/?userId=');
      if (response.data.success) {
        const tokenData = response.data.data || [];
        // 转换数据格式以适应下拉框
        const formattedTokens = tokenData.map(token => ({
          key: token.id,
          text: token.name || t('dashboard.form.unknownToken'),
          value: token.name || ''
        }));
        
        setTokensByNameOptions(formattedTokens);
        setFilteredTokensByNameOptions(formattedTokens);
        setTokenSearchValueByName('');
      }
    } catch (error) {
      console.error('Failed to fetch all tokens:', error);
      setTokensByNameOptions([]);
      setFilteredTokensByNameOptions([]);
    }
  };

  // 获取所有用户Token使用情况
  const fetchAllUserUsageStats = async () => {
    setIsLoading(true);
    try {
      const endDate = new Date();
      const startDate = new Date();
      startDate.setDate(startDate.getDate() - 7); // 默认获取最近一周的数据
      
      const startTimestamp = Math.floor(startDate.getTime() / 1000);
      const endTimestamp = Math.floor(endDate.getTime() / 1000);
      
      const response = await API.get(`/api/log/stats?startTimestamp=${startTimestamp}&endTimestamp=${endTimestamp}`);
      
      if (response.data.success) {
        setUsageStats(response.data.data || []);
      } else {
        console.error('Failed to fetch usage stats:', response.data.message);
        setUsageStats([]);
      }
    } catch (error) {
      console.error('Error fetching usage stats:', error);
      setUsageStats([]);
    } finally {
      setIsLoading(false);
    }
  };

  // 获取特定用户的Token使用统计
  const fetchUserTokenStats = async (userId, tokenName, startTimestamp, endTimestamp) => {
    setIsLoading(true);
    try {
      const response = await API.get(
        `/api/log/user/stats?userId=${userId}&tokenName=${tokenName}&startTimestamp=${startTimestamp}&endTimestamp=${endTimestamp}`
      );
      
      if (response.data.success) {
        setUsageStats(response.data.data || []);
      } else {
        console.error('Failed to fetch user token stats:', response.data.message);
        setUsageStats([]);
      }
    } catch (error) {
      console.error('Error fetching user token stats:', error);
      setUsageStats([]);
    } finally {
      setIsLoading(false);
    }
  };

  // 新增获取数据函数
  const fetchTokenUsageByName = async (startDate, endDate, username, tokenName) => {
    try {
      const start = new Date(startDate);
      const end = new Date(endDate);
      end.setHours(23, 59, 59, 999);  // 设置为当天结束时间
      
      const startTimestamp = Math.floor(start.getTime() / 1000);
      const endTimestamp = Math.floor(end.getTime() / 1000);
      
      const response = await API.get(`/api/log/token/usage-by-name?startTimestamp=${startTimestamp}&endTimestamp=${endTimestamp}${username ? `&userId=${username}` : ''}${tokenName ? `&tokenName=${tokenName}` : ''}`);
      
      if (response.data.success) {
        const rawData = response.data.data || [];
        
        // 按token_name分组处理数据
        const tokenGroups = {};
        const userSet = new Set();
        
        // 第一步：收集所有用户名和token名称
        rawData.forEach(item => {
          const username = item.username || 'unknown';
          userSet.add(username);
          
          if (!tokenGroups[item.token_name]) {
            tokenGroups[item.token_name] = {
              token_name: item.token_name
            };
          }
        });
        
        // 将Set转换为数组，方便后续使用
        const userArray = Array.from(userSet);
        
        // 第二步：为每个token初始化每个用户的使用量为0
        Object.keys(tokenGroups).forEach(tokenName => {
          userArray.forEach(username => {
            tokenGroups[tokenName][username] = 0;
          });
        });
        
        // 第三步：填充实际数据
        rawData.forEach(item => {
          const username = item.username || 'unknown';
          if (tokenGroups[item.token_name]) {
            tokenGroups[item.token_name][username] = parseInt(item.total_tokens) || 0;
            // 存储请求次数
            tokenGroups[item.token_name][`${username}_count`] = parseInt(item.request_count) || 0;
          }
        });
        
        // 转换为数组形式，便于图表使用
        const formattedData = Object.values(tokenGroups);
        
        setTokenUsageByNameData(formattedData);
      } else {
        setTokenUsageByNameData([]);
      }
    } catch (error) {
      console.error('Failed to fetch token usage by name:', error);
      setTokenUsageByNameData([]);
    }
  };

  // 处理表单提交
  const handleTokenUsageByNameSubmit = (e) => {
    e.preventDefault();
    const { startDate, endDate, username, tokenName } = tokenUsageByNameForm;
    
    if (!startDate || !endDate) {
      alert(t('dashboard.form.validation.dateRequired'));
      return;
    }
    
    fetchTokenUsageByName(startDate, endDate, username, tokenName);
  };

  // 修改用户选择处理函数
  const handleTokenUsageByNameUserSelect = (e, { value }) => {
    console.log('用户选择变更:', value); // 添加调试日志
    
    // 当value为null或undefined时，表示用户点击了清除按钮
    if (!value && value !== 0) {
      setTokenUsageByNameForm({
        ...tokenUsageByNameForm,
        username: '',
        userId: '',
        tokenName: '' // 同时清除token选择
      });
      
      // 加载所有token
      fetchAllTokensFromLogs();
      return;
    }
    
    // 正常选择用户的逻辑
    setTokenUsageByNameForm({
      ...tokenUsageByNameForm,
      username: value,
      userId: value,
      tokenName: '' // 重置token选择
    });
    
    // 加载该用户的token列表到ByName表单
    fetchUserTokens(value, 'byName');
  };

  return (
    <div className='dashboard-container'>
      {/* 三个并排的折线图 */}
      <Grid columns={3} stackable className='charts-grid'>
        <Grid.Column>
          <Card fluid className='chart-card'>
            <Card.Content>
              <Card.Header>
                {t('dashboard.charts.requests.title')}
                {/* <span className='stat-value'>{summaryData.todayRequests}</span> */}
              </Card.Header>
              <div className='chart-container'>
                <ResponsiveContainer
                  width='100%'
                  height={120}
                  margin={{ left: 10, right: 10 }} // 调整容器边距
                >
                  <LineChart data={timeSeriesData}>
                    <CartesianGrid
                      strokeDasharray='3 3'
                      vertical={chartConfig.lineChart.grid.vertical}
                      horizontal={chartConfig.lineChart.grid.horizontal}
                      opacity={chartConfig.lineChart.grid.opacity}
                    />
                    <XAxis {...xAxisConfig} />
                    <YAxis hide={true} />
                    <Tooltip
                      contentStyle={{
                        background: '#fff',
                        border: 'none',
                        borderRadius: '4px',
                        boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
                      }}
                      formatter={(value) => [
                        value,
                        t('dashboard.charts.requests.tooltip'),
                      ]}
                      labelFormatter={(label) =>
                        `${t(
                          'dashboard.statistics.tooltip.date'
                        )}: ${formatDate(label)}`
                      }
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

        <Grid.Column>
          <Card fluid className='chart-card'>
            <Card.Content>
              <Card.Header>
                {t('dashboard.charts.quota.title')}
                {/* <span className='stat-value'>
                  ${summaryData.todayQuota.toFixed(3)}
                </span> */}
              </Card.Header>
              <div className='chart-container'>
                <ResponsiveContainer
                  width='100%'
                  height={120}
                  margin={{ left: 10, right: 10 }} // 调整容器边距
                >
                  <LineChart data={timeSeriesData}>
                    <CartesianGrid
                      strokeDasharray='3 3'
                      vertical={chartConfig.lineChart.grid.vertical}
                      horizontal={chartConfig.lineChart.grid.horizontal}
                      opacity={chartConfig.lineChart.grid.opacity}
                    />
                    <XAxis {...xAxisConfig} />
                    <YAxis hide={true} />
                    <Tooltip
                      contentStyle={{
                        background: '#fff',
                        border: 'none',
                        borderRadius: '4px',
                        boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
                      }}
                      formatter={(value) => [
                        value.toFixed(6),
                        t('dashboard.charts.quota.tooltip'),
                      ]}
                      labelFormatter={(label) =>
                        `${t(
                          'dashboard.statistics.tooltip.date'
                        )}: ${formatDate(label)}`
                      }
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

        <Grid.Column>
          <Card fluid className='chart-card'>
            <Card.Content>
              <Card.Header>
                {t('dashboard.charts.tokens.title')}
                {/* <span className='stat-value'>{summaryData.todayTokens}</span> */}
              </Card.Header>
              <div className='chart-container'>
                <ResponsiveContainer
                  width='100%'
                  height={120}
                  margin={{ left: 10, right: 10 }} // 调整容器边距
                >
                  <LineChart data={timeSeriesData}>
                    <CartesianGrid
                      strokeDasharray='3 3'
                      vertical={chartConfig.lineChart.grid.vertical}
                      horizontal={chartConfig.lineChart.grid.horizontal}
                      opacity={chartConfig.lineChart.grid.opacity}
                    />
                    <XAxis {...xAxisConfig} />
                    <YAxis hide={true} />
                    <Tooltip
                      contentStyle={{
                        background: '#fff',
                        border: 'none',
                        borderRadius: '4px',
                        boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
                      }}
                      formatter={(value) => [
                        value,
                        t('dashboard.charts.tokens.tooltip'),
                      ]}
                      labelFormatter={(label) =>
                        `${t(
                          'dashboard.statistics.tooltip.date'
                        )}: ${formatDate(label)}`
                      }
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
                <CartesianGrid
                  strokeDasharray='3 3'
                  vertical={false}
                  opacity={0.1}
                />
                <XAxis {...xAxisConfig} />
                <YAxis
                  axisLine={false}
                  tickLine={false}
                  tick={{ fontSize: 12, fill: '#A3AED0' }}
                />
                <Tooltip
                  contentStyle={{
                    background: '#fff',
                    border: 'none',
                    borderRadius: '4px',
                    boxShadow: '0 2px 8px rgba(0,0,0,0.1)',
                  }}
                  labelFormatter={(label) => {
                    const comDate = label + ':00:00';
                    const date = new Date(comDate);
                    // 只显示日期
                    return `日期: ${date.getMonth() + 1}/${date.getDate()}`;
                  }}
                />
                <Legend
                  wrapperStyle={{
                    paddingTop: '20px',
                  }}
                />
                {models.map((model, index) => (
                  <Bar
                    key={model}
                    dataKey={model}
                    stackId='a'
                    fill={getRandomColor(index)}
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
            <Card.Header style={{ fontSize: '14px', textAlign: 'center' }}>{t('dashboard.tokenUsageByName.chartTitle')}</Card.Header>
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
                    <YAxis
                      axisLine={false}
                      tickLine={false}
                      tick={{ fontSize: 12, fill: '#A3AED0' }}
                    />
                    <Tooltip
                      contentStyle={{
                        background: '#fff',
                        border: 'none',
                        borderRadius: '8px',
                        boxShadow: '0 4px 12px rgba(0,0,0,0.15)',
                        padding: '10px',
                        fontSize: '14px'
                      }}
                      formatter={(value, name, props) => {
                        const displayName = name === 'unknown' ? t('dashboard.tokenStats.unknownUser') : name;
                        // 获取该用户的请求次数
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
                      formatter={(value) => {
                        return value === 'unknown' ? t('dashboard.tokenStats.unknownUser') : value;
                      }}
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
                            fill={getRandomColor(index)}
                            name={username}
                            radius={[4, 4, 0, 0]}
                          />
                        ))
                    }
                  </BarChart>
                </ResponsiveContainer>
              ) : (
                <div style={{ 
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
                }}>
                  {t('dashboard.tokenUsageByName.noDataHint')}
                </div>
              )}
            </div>
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
            <Card.Header style={{ fontSize: '14px', textAlign: 'center' }}>{t('dashboard.tokenModelUsage.chartTitle')}</Card.Header>
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
                  width: '120px',  // 增加宽度
                  height: '34px',  // 固定高度
                  boxShadow: '0 1px 2px rgba(0,0,0,0.05)',
                  appearance: 'none',
                  backgroundImage: 'url("data:image/svg+xml;charset=US-ASCII,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20width%3D%22292.4%22%20height%3D%22292.4%22%3E%3Cpath%20fill%3D%22%23555%22%20d%3D%22M287%2069.4a17.6%2017.6%200%200%200-13-5.4H18.4c-5%200-9.3%201.8-12.9%205.4A17.6%2017.6%200%200%200%200%2082.2c0%205%201.8%209.3%205.4%2012.9l128%20127.9c3.6%203.6%207.8%205.4%2012.8%205.4s9.2-1.8%2012.8-5.4L287%2095c3.5-3.5%205.4-7.8%205.4-12.8%200-5-1.9-9.2-5.5-12.8z%22%2F%3E%3C%2Fsvg%3E")',
                  backgroundRepeat: 'no-repeat',
                  backgroundPosition: 'right 8px center',
                  backgroundSize: '10px 6px',
                  paddingRight: '24px',
                  transition: 'all 0.2s ease',
                  fontWeight: '500',
                  '&:hover': {
                    borderColor: '#bbb',
                    boxShadow: '0 1px 3px rgba(0,0,0,0.1)'
                  }
                }}
              >
                <option value='day'>{t('dashboard.form.timeInterval.day')}</option>
                <option value='hour'>{t('dashboard.form.timeInterval.hour')}</option>
              </select>
            </div>
            <div className='chart-container'>
              {processedData.chartData.length > 0 ? (
                <ResponsiveContainer width='100%' height={500}>
                  <BarChart
                    data={processedData.chartData}
                    barSize={timeInterval === 'hour' ? 8 : 60}
                    margin={{ top: 20, right: 30, left: 20, bottom: 60 }}
                  >
                    <CartesianGrid strokeDasharray='3 3' vertical={false} opacity={0.1} />
                    <XAxis dataKey='interval' {...xAxisIntervalConfig} />
                    <YAxis 
                      axisLine={false} 
                      tickLine={false} 
                      tick={{ fontSize: 14, fill: '#A3AED0' }}
                      width={60}
                    />
                    <Tooltip
                      contentStyle={{
                        background: '#fff',
                        border: 'none',
                        borderRadius: '8px',
                        boxShadow: '0 4px 12px rgba(0,0,0,0.15)',
                        padding: '10px',
                        fontSize: '14px'
                      }}
                      formatter={(value, name, props) => {
                        const displayName = name === 'unknown' ? t('dashboard.tokenStats.unknownUser') : name;
                        // 获取该用户的请求次数
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
                      itemSorter={(item) => {
                        // 按使用量从大到小排序
                        return -item.value;
                      }}
                      labelFormatter={(label) => {
                        const comDate = label + ':00:00';
                        const date = new Date(comDate);
                        
                        // 根据时间间隔显示不同格式的日期
                        if (timeInterval === 'hour') {
                          // 显示具体到小时的时间
                          return `日期: ${date.getMonth() + 1}/${date.getDate()} ${date.getHours()}:00`;
                        } else {
                          // 只显示日期
                          return `日期: ${date.getMonth() + 1}/${date.getDate()}`;
                        }
                      }}
                      wrapperStyle={{
                        zIndex: 1000
                      }}
                    />
                    <Legend 
                      formatter={(value) => {
                        return value === 'unknown' ? t('dashboard.tokenStats.unknownUser') : value;
                      }}
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
                        fill={getRandomColor(index)}
                        name={modelName}
                        radius={[4, 4, 0, 0]}
                      />
                    ))}
                  </BarChart>
                </ResponsiveContainer>
              ) : (
                <div style={{ 
                  height: '200px', 
                  display: 'flex', 
                  alignItems: 'center', 
                  justifyContent: 'center',
                  color: '#999',
                  fontSize: '14px',
                  border: '1px dashed #ddd',
                  borderRadius: '4px',
                  margin: '20px 0',
                  backgroundColor: '#fafafa',
                  pointerEvents: 'none' // 禁用鼠标事件
                }}>
                  {t('dashboard.tokenModelUsage.noDataHint')}
                </div>
              )}
            </div>
          </Card.Content>
        </Card>
      </Grid.Column>

      {/* 用户使用统计表格 */}
      <Grid.Column width={16}>
        <Card fluid className='chart-card'>
          <Card.Content>
            <Card.Header style={{ fontSize: '14px', textAlign: 'center' }}>
              {t('dashboard.tokenModelUsage.tableTitle')}
            </Card.Header>
            
            {isLoading ? (
              <div style={{ textAlign: 'center', padding: '40px 0' }}>
                <div className="ui active centered inline loader"></div>
                <p style={{ marginTop: '10px', color: '#666' }}>{t('dashboard.tokenStats.loading')}</p>
              </div>
            ) : (
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
                    {usageStats.length > 0 ? (
                      usageStats.map((item, index) => (
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
                  {usageStats.length > 0 && (
                    <tfoot>
                      <tr>
                        <th colSpan="2" style={{ textAlign: 'right' }}>{t('dashboard.tokenStats.total')}:</th>
                        <th>
                          {usageStats.reduce((sum, item) => sum + (item.prompt_tokens || 0), 0)}
                        </th>
                        <th>
                          {usageStats.reduce((sum, item) => sum + (item.completion_tokens || 0), 0)}
                        </th>
                        <th>
                          {usageStats.reduce((sum, item) => sum + (item.total_tokens || 0), 0)}
                        </th>
                        <th>
                          {usageStats.reduce((sum, item) => sum + (item.request_count || 0), 0)}
                        </th>
                        <th></th>
                      </tr>
                    </tfoot>
                  )}
                </table>
              </div>
            )}
          </Card.Content>
        </Card>
      </Grid.Column>

      
    </div>
  );
};

export default Dashboard;
