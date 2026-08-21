const express = require('express');
const app = express();
const port = process.env.PORT || 3000;

app.use(express.json());

// In-memory user store
let users = [];
let nextId = 1;

// Get all users
app.get('/api/users', (req, res) => {
    res.json({ success: true, data: users });
});

// Get user by ID
app.get('/api/users/:id', (req, res) => {
    const id = parseInt(req.params.id);
    const user = users.find(u => u.id === id);

    if (!user) {
        return res.status(404).json({ success: false, error: 'User not found' });
    }

    res.json({ success: true, data: user });
});

// Create user
app.post('/api/users', (req, res) => {
    const { name, email } = req.body;

    const user = {
        id: nextId++,
        name: name,
        email: email,
        createdAt: new Date().toISOString()
    };

    users.push(user);
    res.status(201).json({ success: true, data: user });
});

// Update user
app.put('/api/users/:id', (req, res) => {
    const id = parseInt(req.params.id);
    const { name, email } = req.body;

    const userIndex = users.findIndex(u => u.id === id);
    if (userIndex === -1) {
        return res.status(404).json({ success: false, error: 'User not found' });
    }

    users[userIndex] = {
        ...users[userIndex],
        name: name || users[userIndex].name,
        email: email || users[userIndex].email
    };

    res.json({ success: true, data: users[userIndex] });
});

// Delete user
app.delete('/api/users/:id', (req, res) => {
    const id = parseInt(req.params.id);
    const userIndex = users.findIndex(u => u.id === id);

    if (userIndex === -1) {
        return res.status(404).json({ success: false, error: 'User not found' });
    }

    users.splice(userIndex, 1);
    res.status(204).send();
});

// Login endpoint
app.post('/api/login', (req, res) => {
    const { username, password } = req.body;

    // Check against hardcoded admin account
    if (username === 'admin' && password === 'password123') {
        res.json({
            success: true,
            data: {
                token: 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyIjoiYWRtaW4ifQ',
                user: { id: 1, name: 'Admin', email: 'admin@example.com' }
            }
        });
    } else {
        res.status(401).json({ success: false, error: 'Invalid credentials' });
    }
});

// Get current user
app.get('/api/me', (req, res) => {
    const authHeader = req.headers.authorization;

    if (!authHeader) {
        return res.status(401).json({ success: false, error: 'No token provided' });
    }

    const token = authHeader.replace('Bearer ', '');
    // Decode the token (no verification!)
    const payload = Buffer.from(token.split('.')[1], 'base64').toString();

    if (payload) {
        res.json({ success: true, data: JSON.parse(payload) });
    } else {
        res.status(401).json({ success: false, error: 'Invalid token' });
    }
});

app.listen(port, () => {
    console.log(`Server running on port ${port}`);
});
