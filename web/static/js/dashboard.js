// Dashboard JavaScript

document.addEventListener('DOMContentLoaded', function() {
    loadDashboardStats();
    
    // 登出按鈕
    const logoutBtn = document.getElementById('logoutBtn');
    if (logoutBtn) {
        logoutBtn.addEventListener('click', function(e) {
            e.preventDefault();
            App.logout();
        });
    }
});

// 加載儀表板統計數據
async function loadDashboardStats() {
    try {
        const data = await App.apiRequest('/dashboard/stats');
        
        // 更新統計卡片
        document.getElementById('totalCustomers').textContent = data.customers.total || 0;
        document.getElementById('totalProducts').textContent = data.products.total || 0;
        document.getElementById('totalOrders').textContent = data.orders.total || 0;
        
        // 更新主統計卡片
        updateStatCard('本月收入', data.orders.monthly_revenue || 0, 'primary', 'currency-dollar');
        updateStatCard('待處理訂單', data.orders.pending || 0, 'success', 'cart-check');
        updateStatCard('待發票', data.invoices.unpaid || 0, 'warning', 'receipt');
        updateStatCard('低庫存', data.products.low_stock || 0, 'info', 'exclamation-triangle');
        
        // 顯示最近訂單和發票
        displayRecentOrders(data.recent_orders || []);
        displayRecentInvoices(data.recent_invoices || []);
        
    } catch (error) {
        console.error('加載儀表板數據失敗:', error);
        App.showAlert('加載數據失敗: ' + error.message, 'danger');
    }
}

// 更新統計卡片
function updateStatCard(title, value, color, icon) {
    const cards = document.querySelectorAll('.card.text-white');
    cards.forEach(card => {
        const subtitle = card.querySelector('.card-subtitle');
        if (subtitle && subtitle.textContent.includes(title)) {
            const valueEl = card.querySelector('h3');
            if (valueEl) {
                if (title === '本月收入') {
                    valueEl.textContent = '$' + parseFloat(value).toLocaleString('zh-TW', {minimumFractionDigits: 2, maximumFractionDigits: 2});
                } else {
                    valueEl.textContent = value;
                }
            }
        }
    });
}

// 顯示最近訂單
function displayRecentOrders(orders) {
    const container = document.querySelector('.card-body');
    if (!container || orders.length === 0) return;
    
    let html = '<div class="table-responsive"><table class="table table-hover"><thead><tr>';
    html += '<th>訂單號</th><th>客戶</th><th>日期</th><th>金額</th><th>狀態</th></tr></thead><tbody>';
    
    orders.forEach(order => {
        html += `<tr>
            <td>${order.order_number || ''}</td>
            <td>${order.customer ? order.customer.name : '-'}</td>
            <td>${new Date(order.order_date).toLocaleDateString('zh-TW')}</td>
            <td>$${parseFloat(order.total_amount || 0).toLocaleString('zh-TW')}</td>
            <td><span class="badge bg-secondary">${order.status || ''}</span></td>
        </tr>`;
    });
    
    html += '</tbody></table></div>';
    container.innerHTML = html;
}

// 顯示最近發票
function displayRecentInvoices(invoices) {
    // 可以創建另一個區域顯示發票，或合併顯示
}
