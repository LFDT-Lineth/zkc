;;error:4:20-26:recursion not permitted here
(defcolumns (X :i16))
;; recursive :)
(defun (id x) (+ x (id x)))
