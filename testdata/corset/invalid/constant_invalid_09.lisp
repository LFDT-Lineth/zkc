;;error:3:18-19:not permitted in pure context
(defcolumns (A :i16))
(defconst BROKEN A)
