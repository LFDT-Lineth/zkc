;; A simple example which demonstrates how a sorting constraint can be
;; implemented on a column of bytes.

;; Input column
(defcolumns (X :i16@prove))

;; Generated columns
(defcolumns (Delta :i16))
(defcolumns (Byte_0 :i8@prove) (Byte_1 :i8@prove))

;; Ensure Delta is a u16
(defconstraint delta_type () (== Delta (+ (* 256 Byte_1) Byte_0)))

;; Delta == X - X[i-1]
(defconstraint sort () (== Delta (- X (shift X -1))))
