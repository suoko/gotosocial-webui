function toggleReplyForm(postId) {
    var replyForm = document.getElementById('reply-form-' + postId);
    replyForm.style.display = 'block';
}

function submitReply(event, username, postId) {
    event.preventDefault();
    var replyText = document.getElementById('reply-text-' + postId).value;
    fetch('/reply', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({ id: postId, replyText: '@' + username + ' ' + replyText })
    }).then(response => {
        if (response.ok) {
            alert('Replied successfully');
            document.getElementById('reply-text-' + postId).value = '';
            document.getElementById('reply-form-' + postId).style.display = 'none';
        } else {
            alert('Failed to reply');
        }
    });
}

function boost(id) {
    fetch('/boost', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({ id: id })
    }).then(response => {
        if (response.ok) {
            alert('Boosted successfully');
        } else {
            alert('Failed to boost');
        }
    });
}

function favourite(id) {
    fetch('/favourite', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify({ id: id })
    }).then(response => {
        if (response.ok) {
            alert('Favourited successfully');
        } else {
            alert('Failed to favourite');
        }
    });
}
